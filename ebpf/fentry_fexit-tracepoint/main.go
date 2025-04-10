// Copyright 2023 Leon Hwang.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"internal/assert"
	"internal/pkg/bpf"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/florianl/go-tc"
	"github.com/florianl/go-tc/core"
	nl "github.com/mdlayher/netlink"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang tp ./tracepoint.c -- -D__TARGET_ARCH_x86 -I../headers -Wall
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang tpbtf ./tp_btf.c -- -D__TARGET_ARCH_x86 -I../headers -Wall
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang ff ./fentry_fexit.c -- -D__TARGET_ARCH_x86 -I../headers -Wall

func main() {
	var runTpBtf bool
	var withoutTracing bool
	flag.BoolVar(&withoutTracing, "T", false, "without tracing")
	flag.BoolVar(&runTpBtf, "tp_btf", false, "Run tp_btf")
	flag.Parse()

	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatalf("Failed to remove rlimit memlock: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if runTpBtf {
		var obj tpbtfObjects
		assert.NoVerifierErr(loadTpbtfObjects(&obj, nil), "Failed to load tp_btf bpf obj: %v")
		defer obj.Close()

		l, err := link.AttachTracing(link.TracingOptions{
			Program:    obj.TpNetlinkExtack,
			AttachType: ebpf.AttachTraceRawTp,
		})
		assert.NoErr(err, "Failed to attach tracing: %v")
		defer l.Close()

		running(ctx, obj.ErrmsgPb)
		return
	}

	var obj tpObjects
	if err := loadTpObjects(&obj, nil); err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			log.Printf("Failed to load bpf obj: %v\n%-20v", err, ve)
		} else {
			log.Printf("Failed to load bpf obj: %v", err)
		}
		return
	}
	defer obj.Close()

	spec, err := loadFf()
	if err != nil {
		log.Printf("Failed to load bpf obj: %v", err)
		return
	}

	funcName, err := bpf.GetProgEntryFuncName(obj.TpNetlinkExtack)
	if err != nil {
		funcName = "tp__netlink_extack"
		log.Printf("Failed to get func name: %v. Use %s instead", err, funcName)
	}

	tpFentry := spec.Programs["fentry_netlink_extack"]
	tpFentry.AttachTarget = obj.TpNetlinkExtack
	tpFentry.AttachTo = funcName
	tpExit := spec.Programs["fexit_netlink_extack"]
	tpExit.AttachTarget = obj.TpNetlinkExtack
	tpExit.AttachTo = funcName

	var ffObj ffObjects
	if err := spec.LoadAndAssign(&ffObj, &ebpf.CollectionOptions{
		MapReplacements: map[string]*ebpf.Map{
			"errmsg_pb": obj.ErrmsgPb,
		},
	}); err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			log.Printf("Failed to load bpf obj: %v\n%-20v", err, ve)
		} else {
			log.Printf("Failed to load bpf obj: %v", err)
		}
		return
	}
	defer ffObj.Close()

	if !withoutTracing {
		if link, err := link.AttachTracing(link.TracingOptions{
			Program: ffObj.FentryNetlinkExtack,
		}); err != nil {
			log.Printf("Failed to attach fentry(netlink_extack): %v", err)
			return
		} else {
			defer link.Close()
			log.Printf("Attached fentry(netlink_extack)")
		}

		if link, err := link.AttachTracing(link.TracingOptions{
			Program: ffObj.FexitNetlinkExtack,
		}); err != nil {
			log.Printf("Failed to attach fexit(netlink_extack): %v", err)
			return
		} else {
			defer link.Close()
			log.Printf("Attached fexit(netlink_extack)")
		}
	}

	if tp, err := link.Tracepoint("netlink", "netlink_extack", obj.TpNetlinkExtack, nil); err != nil {
		log.Printf("Failed to attach tracepoint(netlink_extack): %v", err)
		return
	} else {
		log.Printf("Attached tracepoint(netlink_extack)")
		defer tp.Close()
	}

	running(ctx, obj.ErrmsgPb)
}

func running(ctx context.Context, m *ebpf.Map) {
	errg, ctx := errgroup.WithContext(ctx)
	errg.Go(func() error {
		handlePerfEvent(ctx, m)
		return nil
	})

	errg.Go(func() error {
		return runTc(ctx)
	})

	<-ctx.Done()

	if err := errg.Wait(); err != nil {
		log.Printf("Error: %v", err)
	}
}

func handlePerfEvent(ctx context.Context, events *ebpf.Map) error {
	eventReader, err := perf.NewReader(events, 4096)
	if err != nil {
		log.Printf("Failed to create perf-event reader: %v", err)
		return fmt.Errorf("create perf-event reader: %w", err)
	}

	log.Printf("Listening events...")

	go func() {
		<-ctx.Done()
		eventReader.Close()
	}()

	var ev struct {
		Msg       [64]byte
		Len       uint16
		ProbeType uint8
		Retval    uint8
	}
	for {
		event, err := eventReader.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				return nil
			}

			log.Printf("Reading perf-event: %v", err)
		}

		if event.LostSamples != 0 {
			log.Printf("Lost %d events", event.LostSamples)
		}

		binary.Read(bytes.NewBuffer(event.RawSample), binary.LittleEndian, &ev)

		switch ev.ProbeType {
		default:
			log.Printf("Errmsg: %s (tracepoint)", ev.Msg[:min(64, ev.Len)])
		case 1:
			log.Printf("Errmsg: %s (fentry)", ev.Msg[:min(64, ev.Len)])
		case 2:
			log.Printf("Errmsg: %s (fexit: %d)", ev.Msg[:min(64, ev.Len)], ev.Retval)
		}

		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
}

func newTcProg() (*ebpf.Program, error) {
	var spec ebpf.ProgramSpec
	spec.Type = ebpf.SchedCLS
	spec.Instructions = asm.Instructions{
		asm.Mov.Imm(asm.R0, 0),
		asm.Return(),
	}

	return ebpf.NewProgram(&spec)
}

func newTcBpfObj(ifindex uint32, prog *ebpf.Program) *tc.Object {
	progFD := uint32(prog.FD())

	flags := uint32(tc.BpfActDirect)
	protocol := htons(unix.ETH_P_ALL)
	direction := core.BuildHandle(tc.HandleRoot, tc.HandleMinIngress)

	annotation := "fake"

	return &tc.Object{
		Msg: tc.Msg{
			Family:  unix.AF_UNSPEC,
			Ifindex: ifindex,
			Handle:  0xfffffff1,
			Parent:  direction,
			Info:    100<<16 | uint32(protocol),
		},
		Attribute: tc.Attribute{
			Kind: "bpf",
			BPF: &tc.Bpf{
				FD:    &progFD,
				Name:  &annotation,
				Flags: &flags,
			},
		},
	}
}

func htons(i uint16) uint16 {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, i)
	return *(*uint16)(unsafe.Pointer(&b[0]))
}

func openTc() (*tc.Tc, error) {
	rtnl, err := tc.Open(&tc.Config{})
	if err != nil {
		return nil, err
	}

	if err := rtnl.SetOption(nl.ExtendedAcknowledge, true); err != nil {
		_ = rtnl.Close()
		return nil, fmt.Errorf("set extended acknowledge: %w", err)
	}

	return rtnl, nil
}

func setTcFilter(rtnl *tc.Tc, obj *tc.Object) error {
	if err := rtnl.Filter().Replace(obj); err != nil {
		return fmt.Errorf("filter replace: %w", err)
	}

	return nil
}

func runTc(ctx context.Context) error {
	prog, err := newTcProg()
	if err != nil {
		return fmt.Errorf("new tc prog: %w", err)
	}
	defer prog.Close()

	rtnl, err := openTc()
	if err != nil {
		return err
	}
	defer rtnl.Close()

	tcObj := newTcBpfObj(1, prog)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	log.Printf("Keep setting tc filter...")

	for {
		select {
		case <-ticker.C:
			err := setTcFilter(rtnl, tcObj)
			if err != nil {
				log.Printf("set tc filter: %v", err)
			} else {
				log.Printf("set tc filter")
			}

		case <-ctx.Done():
			return nil
		}
	}
}
