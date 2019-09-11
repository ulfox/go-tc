//+build linux

package tc

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func TestQdisc(t *testing.T) {
	tcSocket, done := testConn(t)
	defer done()

	err := tcSocket.Qdisc().Add(nil)
	if err != ErrNoArg {
		t.Fatalf("expected ErrNoArg, received: %v", err)
	}

	tests := map[string]struct {
		kind    string
		fqCodel *FqCodel
		red     *Red
		sfb     *Sfb
		cbq     *Cbq
		codel   *Codel
		hhf     *Hhf
		pie     *Pie
	}{
		"clsact":   {kind: "clsact"},
		"fq_codel": {kind: "fq_codel", fqCodel: &FqCodel{Target: 42, Limit: 0xCAFE}},
		"red":      {kind: "red", red: &Red{MaxP: 42}},
		"sfb":      {kind: "sfb", sfb: &Sfb{Parms: &SfbQopt{Max: 0xFF}}},
		"cbq":      {kind: "cbq", cbq: &Cbq{LssOpt: &CbqLssOpt{OffTime: 10}, WrrOpt: &CbqWrrOpt{Weight: 42}, FOpt: &CbqFOpt{Split: 2}, OVLStrategy: &CbqOvl{Penalty: 2}}},
		"codel":    {kind: "codel", codel: &Codel{Target: 1, Limit: 2, Interval: 3, ECN: 4, CEThreshold: 5}},
		"hhf":      {kind: "hhf", hhf: &Hhf{BacklogLimit: 1, Quantum: 2, HHFlowsLimit: 3, ResetTimeout: 4, AdmitBytes: 5, EVICTTimeout: 6, NonHHWeight: 7}},
		"pie":      {kind: "pie", pie: &Pie{Target: 1, Limit: 2, TUpdate: 3, Alpha: 4, Beta: 5, ECN: 6, Bytemode: 7}},
	}

	tcMsg := Msg{
		Family:  unix.AF_UNSPEC,
		Ifindex: 123,
		Handle:  BuildHandle(0xFFFF, 0x0000),
		Parent:  0xFFFFFFF1,
		Info:    0,
	}
	for name, testcase := range tests {
		t.Run(name, func(t *testing.T) {

			testQdisc := Object{
				tcMsg,
				Attribute{
					Kind:    testcase.kind,
					FqCodel: testcase.fqCodel,
					Red:     testcase.red,
					Sfb:     testcase.sfb,
					Cbq:     testcase.cbq,
					Codel:   testcase.codel,
					Hhf:     testcase.hhf,
					Pie:     testcase.pie,
				},
			}

			if err := tcSocket.Qdisc().Add(&testQdisc); err != nil {
				t.Fatalf("could not add new qdisc: %v", err)
			}

			qdiscs, err := tcSocket.Qdisc().Get()
			if err != nil {
				t.Fatalf("could not get qdiscs: %v", err)
			}
			for _, qdisc := range qdiscs {
				t.Logf("%#v\n", qdisc)
			}

			if err := tcSocket.Qdisc().Delete(&testQdisc); err != nil {
				t.Fatalf("could not delete qdisc: %v", err)
			}

		})
	}

}

func qdiscAlterResponses(t *testing.T, cache *[]netlink.Message) []byte {
	t.Helper()
	var tmp []Object
	var dataStream []byte

	// Decode data from cache
	for _, msg := range *cache {
		var result Object
		if err := extractTcmsgAttributes(msg.Data[20:], &result.Attribute); err != nil {
			t.Fatalf("could not decode attributes: %v", err)
		}
		tmp = append(tmp, result)
	}

	var stats2 bytes.Buffer
	if err := binary.Write(&stats2, nativeEndian, &Stats2{
		Bytes:      42,
		Packets:    1,
		Qlen:       1,
		Backlog:    0,
		Drops:      0,
		Requeues:   0,
		Overlimits: 42,
	}); err != nil {
		t.Fatalf("could not encode stats2: %v", err)
	}

	var stats bytes.Buffer
	if err := binary.Write(&stats, nativeEndian, &Stats{
		Bytes:      32,
		Packets:    1,
		Drops:      0,
		Overlimits: 0,
		Bps:        1,
		Pps:        1,
		Qlen:       1,
		Backlog:    0,
	}); err != nil {
		t.Fatalf("could not encode stats: %v", err)
	}

	// Alter and marshal data
	for _, obj := range tmp {
		var data []byte
		var attrs []tcOption
		attrs = append(attrs, tcOption{Interpretation: vtString, Type: tcaKind, Data: obj.Kind})
		attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaStats2, Data: stats2.Bytes()})
		attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaStats, Data: stats.Bytes()})
		attrs = append(attrs, tcOption{Interpretation: vtUint8, Type: tcaHwOffload, Data: uint8(0)})

		// add XStats
		switch obj.Kind {
		case "fq_codel":
			data, err := marshalXStats(XStats{FqCodel: &FqCodelXStats{Type: 0, Qd: &FqCodelQdStats{}}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "sfq":
			data, err := marshalXStats(XStats{Sfq: &SfqXStats{Allot: 42}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "sfb":
			data, err := marshalXStats(XStats{Sfb: &SfbXStats{EarlyDrop: 1, PenaltyDrop: 2, AvgProb: 42}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "red":
			data, err := marshalXStats(XStats{Red: &RedXStats{Early: 1, PDrop: 2, Other: 3, Marked: 4}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "choke":
			data, err := marshalXStats(XStats{Choke: &ChokeXStats{Early: 1, PDrop: 2, Other: 3, Marked: 4, Matched: 5}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "htb":
			data, err := marshalXStats(XStats{Htb: &HtbXStats{Lends: 1, Borrows: 2, Giants: 3}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "cbq":
			data, err := marshalXStats(XStats{Cbq: &CbqXStats{Borrows: 2}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "codel":
			data, err := marshalXStats(XStats{Codel: &CodelXStats{MaxPacket: 3, LDelay: 5}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "hhf":
			data, err := marshalXStats(XStats{Hhf: &HhfXStats{DropOverlimit: 42}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		case "pie":
			data, err := marshalXStats(XStats{Pie: &PieXStats{Prob: 2, Delay: 4, Dropped: 5}})
			if err != nil {
				t.Fatalf("could not marshal Xstats struct: %v", err)
			}
			attrs = append(attrs, tcOption{Interpretation: vtBytes, Type: tcaXstats, Data: data})
		}

		marshaled, err := marshalAttributes(attrs)
		if err != nil {
			t.Fatalf("could not marshal attributes: %v", err)
		}
		data = append(data, marshaled...)

		dataStream = append(dataStream, data...)

	}
	return dataStream
}
