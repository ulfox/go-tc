package tc

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/mdlayher/netlink"
)

const (
	tcaU32Unspec = iota
	tcaU32ClassID
	tcaU32Hash
	tcaU32Link
	tcaU32Divisor
	tcaU32Sel
	tcaU32Police
	tcaU32Act
	tcaU32InDev
	tcaU32Pcnt
	tcaU32Mark
	tcaU32Flags
	tcaU32Pad
)

// U32 contains attributes of the u32 discipline
type U32 struct {
	ClassID uint32
	Hash    uint32
	Link    uint32
	Divisor uint32
	Sel     *U32Sel
	InDev   string
	Pcnt    uint64
	Mark    *U32Mark
	Flags   uint32
	Police  *Police
	Actions *[]*Action
}

// marshalU32 returns the binary encoding of U32
func marshalU32(info *U32) ([]byte, error) {
	options := []tcOption{}

	if info == nil {
		return []byte{}, fmt.Errorf("U32: %w", ErrNoArg)
	}

	// TODO: improve logic and check combinations

	if info.Sel != nil {
		data, err := validateU32SelOptions(info.Sel)
		if err != nil {
			return []byte{}, err
		}
		options = append(options, tcOption{Interpretation: vtBytes, Type: tcaU32Sel, Data: data})
	}

	if info.Mark != nil {
		data, err := marshalStruct(info.Mark)
		if err != nil {
			return []byte{}, err
		}
		options = append(options, tcOption{Interpretation: vtBytes, Type: tcaU32Mark, Data: data})
	}

	if info.ClassID != 0 {
		options = append(options, tcOption{Interpretation: vtUint32, Type: tcaU32ClassID, Data: info.ClassID})
	}
	if info.Police != nil {
		data, err := marshalPolice(info.Police)
		if err != nil {
			return []byte{}, err
		}
		options = append(options, tcOption{Interpretation: vtBytes, Type: tcaU32Police, Data: data})
	}
	if info.Flags != 0 {
		options = append(options, tcOption{Interpretation: vtUint32, Type: tcaU32Flags, Data: info.Flags})
	}
	if info.Actions != nil {
		data, err := marshalActions(*info.Actions)
		if err != nil {
			return []byte{}, err
		}
		options = append(options, tcOption{Interpretation: vtBytes, Type: tcaU32Act, Data: data})
	}

	return marshalAttributes(options)
}

// unmarshalU32 parses the U32-encoded data and stores the result in the value pointed to by info.
func unmarshalU32(data []byte, info *U32) error {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return err
	}
	ad.ByteOrder = nativeEndian
	for ad.Next() {
		switch ad.Type() {
		case tcaU32ClassID:
			info.ClassID = ad.Uint32()
		case tcaU32Hash:
			info.Hash = ad.Uint32()
		case tcaU32Link:
			info.Link = ad.Uint32()
		case tcaU32Divisor:
			info.Divisor = ad.Uint32()
		case tcaU32Sel:
			arg := &U32Sel{}
			if err := extractU32Sel(ad.Bytes(), arg); err != nil {
				return err
			}
			info.Sel = arg
		case tcaU32Police:
			pol := &Police{}
			if err := unmarshalPolice(ad.Bytes(), pol); err != nil {
				return err
			}
			info.Police = pol
		case tcaU32InDev:
			info.InDev = ad.String()
		case tcaU32Pcnt:
			info.Pcnt = ad.Uint64()
		case tcaU32Mark:
			arg := &U32Mark{}
			if err := unmarshalStruct(ad.Bytes(), arg); err != nil {
				return err
			}
			info.Mark = arg
		case tcaU32Flags:
			info.Flags = ad.Uint32()
		case tcaU32Act:
			actions := &[]*Action{}
			if err := unmarshalActions(ad.Bytes(), actions); err != nil {
				return err
			}
			info.Actions = actions
		case tcaU32Pad:
			// padding does not contain data, we just skip it
		default:
			return fmt.Errorf("unmarshalU32()\t%d\n\t%v", ad.Type(), ad.Bytes())
		}
	}
	return nil
}

// U32Sel from include/uapi/linux/pkt_sched.h
type U32Sel struct {
	Flags    uint8
	Offshift uint8
	NKeys    uint8
	OffMask  uint16
	Off      uint16
	Offoff   uint16
	Hoff     uint16
	Hmask    uint32
	Keys     []U32Key
}

func validateU32SelOptions(info *U32Sel) ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, nativeEndian, info.Flags)
	binary.Write(buf, nativeEndian, info.Offshift)
	binary.Write(buf, nativeEndian, info.NKeys)
	binary.Write(buf, nativeEndian, info.OffMask)
	binary.Write(buf, nativeEndian, info.Off)
	binary.Write(buf, nativeEndian, info.Offoff)
	binary.Write(buf, nativeEndian, info.Hoff)
	binary.Write(buf, nativeEndian, info.Hmask)
	for _, v := range info.Keys {
		data, err := marshalStruct(v)
		if err != nil {
			return []byte{}, err
		}
		buf.Write(data)
	}
	return buf.Bytes(), nil
}

func extractU32Sel(data []byte, info *U32Sel) error {
	if len(data) < 15 {
		return fmt.Errorf("not enough bytes for U32Sel")
	}
	info.Flags = data[0]
	info.Offshift = data[1]
	info.NKeys = data[2]
	info.OffMask = nativeEndian.Uint16(data[3:5])
	info.Off = nativeEndian.Uint16(data[5:7])
	info.Offoff = nativeEndian.Uint16(data[7:9])
	info.Hoff = nativeEndian.Uint16(data[9:11])
	info.Hmask = nativeEndian.Uint32(data[11:15])
	if len(data) < int(info.NKeys)*16+15 {
		return fmt.Errorf("not enough bytes for U32Keys")
	}
	for i := 0; i < int(info.NKeys); i++ {
		key := &U32Key{}
		if err := unmarshalStruct(data[15+i*16:15+(i+1)*16], key); err != nil {
			return err
		}
		info.Keys = append(info.Keys, *key)
	}
	return nil
}

//U32Mark from include/uapi/linux/pkt_sched.h
type U32Mark struct {
	Val     uint32
	Mask    uint32
	Success uint32
}

// U32Key from include/uapi/linux/pkt_sched.h
type U32Key struct {
	Mask    uint32
	Val     uint32
	Off     uint32
	OffMask uint32
}
