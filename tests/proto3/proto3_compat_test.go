// +build extensive_tests

// only built if manually enforced (via the build tag above)
package proto3

import (
	"bufio"
	"bytes"
	"encoding/binary"

	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptypes "github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/proto"

	p3 "github.com/tendermint/go-amino/tests/proto3/proto"

	"github.com/tendermint/go-amino"
	"github.com/tendermint/go-amino/tests"
)

// This file checks basic proto3 compatibility by checking encoding of some test-vectors generated by using protoc.

var cdc = amino.NewCodec()
var epoch time.Time

func init() {
	cdc.Seal()
	epoch, _ = time.Parse("2006-01-02 15:04:05 +0000 UTC", "1970-01-01 00:00:00 +0000 UTC")
}

func TestFixed32Roundtrip(t *testing.T) {
	// amino fixed32 (int32) <-> protbuf fixed32 (uint32)
	type testi32 struct {
		Int32 int32 `binary:"fixed32"`
	}
	ab, err := cdc.MarshalBinaryBare(testi32{Int32: 150})
	assert.NoError(t, err, "unexpected error")

	pb, err := proto.Marshal(&p3.TestInt32Fixed{Fixed32: 150})
	assert.NoError(t, err, "unexpected error")

	assert.Equal(t, pb, ab, "fixed32 (int32) encoding doesn't match")

	// unmarshal (from amino to proto and vice versa)
	var att testi32
	var pt p3.Test32
	err = proto.Unmarshal(ab, &pt)
	assert.NoError(t, err, "unexpected error")

	err = cdc.UnmarshalBinaryBare(pb, &att)
	assert.NoError(t, err, "unexpected error")

	assert.Equal(t, uint32(att.Int32), pt.Foo)
}

func TestVarintZigzagRoundtrip(t *testing.T) {
	t.Skip("zigzag encoding isn't default anymore for (unsigned) ints")
	// amino varint (int) <-> protobuf zigzag32 (int32 in go sint32 in proto file)
	type testInt32Varint struct {
		Int32 int `binary:"varint"`
	}
	varint := testInt32Varint{Int32: 6000000}
	ab, err := cdc.MarshalBinaryBare(varint)
	assert.NoError(t, err, "unexpected error")
	pb, err := proto.Marshal(&p3.TestInt32Varint{Int32: 6000000})
	assert.NoError(t, err, "unexpected error")
	assert.Equal(t, pb, ab, "varint encoding doesn't match")

	var amToP3 p3.TestInt32Varint
	var p3ToAm testInt32Varint
	err = proto.Unmarshal(ab, &amToP3)
	assert.NoError(t, err, "unexpected error")

	err = cdc.UnmarshalBinaryBare(pb, &p3ToAm)
	assert.NoError(t, err, "unexpected error")

	assert.EqualValues(t, varint.Int32, amToP3.Int32)
}

func TestFixedU64Roundtrip(t *testing.T) {
	type testFixed64Uint struct {
		Int64 uint64 `binary:"fixed64"`
	}

	pvint64 := p3.TestFixedInt64{Int64: 150}
	avint64 := testFixed64Uint{Int64: 150}
	ab, err := cdc.MarshalBinaryBare(avint64)
	assert.NoError(t, err, "unexpected error")

	pb, err := proto.Marshal(&pvint64)
	assert.NoError(t, err, "unexpected error")

	assert.Equal(t, pb, ab, "fixed64 encoding doesn't match")

	var amToP3 p3.TestFixedInt64
	var p3ToAm testFixed64Uint
	err = proto.Unmarshal(ab, &amToP3)
	assert.NoError(t, err, "unexpected error")

	err = cdc.UnmarshalBinaryBare(pb, &p3ToAm)
	assert.NoError(t, err, "unexpected error")

	assert.EqualValues(t, p3ToAm.Int64, amToP3.Int64)
}

func TestMultidimensionalSlices(t *testing.T) {
	s := [][]int8{
		{1, 2},
		{3, 4, 5}}

	_, err := cdc.MarshalBinaryBare(s)
	assert.Error(t, err, "expected error: multidimensional slices are not allowed")
}

func TestMultidimensionalArrays(t *testing.T) {
	arr := [2][2]int8{
		{1, 2},
		{3, 4}}

	_, err := cdc.MarshalBinaryBare(arr)
	assert.Error(t, err, "expected error: multidimensional arrays are not allowed")
}

func TestMultidimensionalByteArraysAndSlices(t *testing.T) {
	arr := [2][2]byte{
		{1, 2},
		{3, 4}}

	_, err := cdc.MarshalBinaryBare(arr)
	assert.NoError(t, err, "unexpected error: multidimensional arrays are allowed, as long as they are only of bytes")

	s := [][]byte{
		{1, 2},
		{3, 4, 5}}

	_, err = cdc.MarshalBinaryBare(s)
	assert.NoError(t, err, "unexpected error: multidimensional slices are allowed, as long as they are only of bytes")

	s2 := [][][]byte{{
		{1, 2},
		{3, 4, 5}}}

	_, err = cdc.MarshalBinaryBare(s2)
	assert.NoError(t, err, "unexpected error: multidimensional slices are allowed, as long as they are only of bytes")

}

func TestProto3CompatPtrsRoundtrip(t *testing.T) {
	s := p3.SomeStruct{}

	ab, err := cdc.MarshalBinaryBare(s)
	assert.NoError(t, err)

	pb, err := proto.Marshal(&s)
	assert.NoError(t, err)
	// This fails as amino currently returns []byte(nil)
	// while protobuf returns []byte{}:
	//
	// assert.Equal(t, ab, pb)
	//
	// Semantically, that's no problem though. Hence, we only check for zero length:
	assert.Zero(t, len(ab), "expected an empty encoding for a nil pointer")
	t.Log(ab)

	var amToP3 p3.SomeStruct
	var p3ToAm p3.SomeStruct
	err = proto.Unmarshal(ab, &amToP3)
	assert.NoError(t, err, "unexpected error")

	err = cdc.UnmarshalBinaryBare(pb, &p3ToAm)
	assert.NoError(t, err, "unexpected error")

	assert.EqualValues(t, p3ToAm, amToP3)

	s2 := p3.SomeStruct{Emb: &p3.EmbeddedStruct{}}

	ab, err = cdc.MarshalBinaryBare(s2)
	assert.NoError(t, err)

	pb, err = proto.Marshal(&s2)
	assert.NoError(t, err)
	assert.Equal(t, ab, pb)

	err = proto.Unmarshal(ab, &amToP3)
	assert.NoError(t, err, "unexpected error")

	err = cdc.UnmarshalBinaryBare(pb, &p3ToAm)
	assert.NoError(t, err, "unexpected error")

	assert.EqualValues(t, p3ToAm, amToP3)

	assert.NotZero(t, len(ab), "expected a non-empty encoding for a non-nil pointer to an empty struct")
	t.Log(ab)

}

// ---------------------------------------------------------------
//  ---- time.Time <-> timestamp.Timestamp (proto3 well known type) :
// ---------------------------------------------------------------

// equivalent go struct or "type" to the proto3 message:
type goAminoGotTime struct {
	T *time.Time
}

func TestProto3CompatEmptyTimestamp(t *testing.T) {
	empty := p3.ProtoGotTime{}
	// protobuf also marshals to empty bytes here:
	pb, err := proto.Marshal(&empty)
	assert.NoError(t, err)
	assert.Len(t, pb, 0)

	// unmarshaling an empty slice behaves a bit differently in proto3 compared to amino:
	res := &goAminoGotTime{}
	err = cdc.UnmarshalBinaryBare(pb, res)
	assert.NoError(t, err)
	// NOTE: this behaves differently because amino defaults the time to 1970-01-01 00:00:00 +0000 UTC while
	// decoding; protobuf defaults to nil here (see the following lines below):
	assert.NoError(t, err)
	assert.Equal(t, goAminoGotTime{T: &epoch}, *res)
	pbRes := p3.ProtoGotTime{}
	err = proto.Unmarshal(pb, &pbRes)
	assert.NoError(t, err)
	assert.Equal(t, p3.ProtoGotTime{T: nil}, pbRes)
}

func TestProto3CompatTimestampNow(t *testing.T) {
	// test with current time:
	now := time.Now()
	ptts, err := ptypes.TimestampProto(now)
	assert.NoError(t, err)
	pt := p3.ProtoGotTime{T: ptts}
	at := goAminoGotTime{T: &now}
	ab1, err := cdc.MarshalBinaryBare(at)
	assert.NoError(t, err)
	ab2, err := cdc.MarshalBinaryBare(pt)
	assert.NoError(t, err)
	// amino's encoding of time.Time is the same as proto's encoding of the well known type
	// timestamp.Timestamp (they can be used interchangeably):
	assert.Equal(t, ab1, ab2)
	pb, err := proto.Marshal(&pt)
	assert.NoError(t, err)
	assert.Equal(t, ab1, pb)

	pbRes := p3.ProtoGotTime{}
	err = proto.Unmarshal(ab1, &pbRes)
	assert.NoError(t, err)
	got, err := ptypes.TimestampFromProto(pbRes.T)
	assert.NoError(t, err)
	_, err = ptypes.TimestampProto(now)
	assert.NoError(t, err)
	err = proto.Unmarshal(pb, &pbRes)
	assert.NoError(t, err)
	// create time.Time from timestamp.Timestamp and check if they are the same:
	got, err = ptypes.TimestampFromProto(pbRes.T)
	assert.Equal(t, got.UTC(), now.UTC())
}

func TestProto3EpochTime(t *testing.T) {
	pbRes := p3.ProtoGotTime{}
	// amino encode epoch (1970) and decode using proto; expect the resulting time to be epoch again:
	ab, err := cdc.MarshalBinaryBare(goAminoGotTime{T: &epoch})
	assert.NoError(t, err)
	err = proto.Unmarshal(ab, &pbRes)
	assert.NoError(t, err)
	ts, err := ptypes.TimestampFromProto(pbRes.T)
	assert.NoError(t, err)
	assert.EqualValues(t, ts, epoch)
}

func TestProtoNegativeSeconds(t *testing.T) {
	pbRes := p3.ProtoGotTime{}
	// test with negative seconds (0001-01-01 -> seconds = -62135596800, nanos = 0):
	ntm, err := time.Parse("2006-01-02 15:04:05 +0000 UTC", "0001-01-01 00:00:00 +0000 UTC")
	ab, err := cdc.MarshalBinaryBare(goAminoGotTime{T: &ntm})
	assert.NoError(t, err)
	res := &goAminoGotTime{}
	err = cdc.UnmarshalBinaryBare(ab, res)
	assert.NoError(t, err)
	assert.EqualValues(t, ntm, *res.T)
	err = proto.Unmarshal(ab, &pbRes)
	assert.NoError(t, err)
	got, err := ptypes.TimestampFromProto(pbRes.T)
	assert.NoError(t, err)
	assert.Equal(t, got, ntm)
}

func TestIntVarintCompat(t *testing.T) {

	tcs := []struct {
		val32 int32
		val64 int64
	}{
		{1, 1},
		{-1, -1},
		{2, 2},
		{1000, 1000},
		{math.MaxInt32, math.MaxInt64},
		{math.MinInt32, math.MinInt64},
	}
	for _, tc := range tcs {
		tv := p3.TestInts{Int32: tc.val32, Int64: tc.val64}
		ab, err := cdc.MarshalBinaryBare(tv)
		assert.NoError(t, err)
		pb, err := proto.Marshal(&tv)
		assert.NoError(t, err)
		assert.Equal(t, ab, pb)
		var res p3.TestInts
		err = cdc.UnmarshalBinaryBare(pb, &res)
		assert.NoError(t, err)
		var res2 p3.TestInts
		err = proto.Unmarshal(ab, &res2)
		assert.NoError(t, err)
		assert.Equal(t, res.Int32, tc.val32)
		assert.Equal(t, res.Int64, tc.val64)
		assert.Equal(t, res2.Int32, tc.val32)
		assert.Equal(t, res2.Int64, tc.val64)
	}
	// special case: amino allows int as well
	// test that ints are also varint encoded:
	type TestInt struct {
		Int int
	}
	tcs2 := []struct {
		val int
	}{
		{0},
		{-1},
		{1000},
		{-1000},
		{math.MaxInt32},
		{math.MinInt32},
	}
	for _, tc := range tcs2 {
		ptv := p3.TestInts{Int32: int32(tc.val)}
		pb, err := proto.Marshal(&ptv)
		assert.NoError(t, err)
		atv := TestInt{tc.val}
		ab, err := cdc.MarshalBinaryBare(atv)
		assert.NoError(t, err)
		if tc.val == 0 {
			// amino results in []byte(nil)
			// protobuf in []byte{}
			assert.Empty(t, ab)
			assert.Empty(t, pb)
		} else {
			assert.Equal(t, ab, pb)
		}
		// can we get back the int from the proto?
		var res TestInt
		err = cdc.UnmarshalBinaryBare(pb, &res)
		assert.NoError(t, err)
		assert.EqualValues(t, res.Int, tc.val)
	}

	// purposely overflow by writing a too large value to first field (which is int32):
	fieldNum := 1
	fieldNumAndType := (uint64(fieldNum) << 3) | uint64(amino.Typ3Varint)
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	var buf [10]byte
	n := binary.PutUvarint(buf[:], fieldNumAndType)
	_, err := writer.Write(buf[0:n])
	assert.NoError(t, err)
	amino.EncodeUvarint(writer, math.MaxInt32+1)
	err = writer.Flush()
	assert.NoError(t, err)

	var res p3.TestInts
	err = cdc.UnmarshalBinaryBare(b.Bytes(), &res)
	assert.Error(t, err)
}

// See if encoding of def types matches the proto3 encoding
func TestTypeDefCompatibility(t *testing.T) {

	pNow := ptypes.TimestampNow()
	now, err := ptypes.TimestampFromProto(pNow)
	require.NoError(t, err)

	strSl := tests.PrimitivesStructSl{
		{Int32: 1, Int64: -1, Varint: 2, String: "protobuf3", Bytes: []byte("got some bytes"), Time: now},
		{Int32: 0, Int64: 1, Varint: -2, String: "amino", Bytes: []byte("more of these bytes"), Time: now},
	}
	strAr := tests.PrimitivesStructAr{strSl[0], strSl[1]}
	p3StrSl := &p3.PrimitivesStructSl{Structs: []*p3.PrimitivesStruct{
		{Int32: 1, Int64: -1, Varint: 2, String_: "protobuf3", Bytes: []byte("got some bytes"), Time: pNow},
		{Int32: 0, Int64: 1, Varint: -2, String_: "amino", Bytes: []byte("more of these bytes"), Time: pNow}},
	}

	tcs := []struct {
		AminoType interface{}
		ProtoMsg  proto.Message
	}{
		// type IntDef int
		0: {tests.IntDef(0), &p3.IntDef{}},
		1: {tests.IntDef(0), &p3.IntDef{Val: 0}},
		2: {tests.IntDef(1), &p3.IntDef{Val: 1}},
		3: {tests.IntDef(-1), &p3.IntDef{Val: -1}},

		// type IntAr [4]int
		4: {tests.IntAr{1, 2, 3, 4}, &p3.IntArr{Val: []int64{1, 2, 3, 4}}},
		5: {tests.IntAr{0, -2, 3, 4}, &p3.IntArr{Val: []int64{0, -2, 3, 4}}},

		// type IntSl []int (protobuf doesn't really have arrays)
		6: {tests.IntSl{1, 2, 3, 4}, &p3.IntArr{Val: []int64{1, 2, 3, 4}}},

		// type PrimitivesStructSl []PrimitivesStruct
		7: {strSl, p3StrSl},
		// type PrimitivesStructAr [2]PrimitivesStruct
		8: {strAr, p3StrSl},
	}
	for i, tc := range tcs {
		ab, err := amino.MarshalBinaryBare(tc.AminoType)
		require.NoError(t, err)

		pb, err := proto.Marshal(tc.ProtoMsg)
		require.NoError(t, err)

		assert.Equal(t, pb, ab, "Amino and protobuf encoding do not match %v", i)
	}
}

// See if encoding of a registered type matches the proto3 encoding
func TestRegisteredTypesCompatibilitySimple(t *testing.T) {
	type message interface{}

	const name = "simpleMsg"
	type simpleMsg struct {
		Message string
		Height  int
	}

	type simpleMsgUnregistered struct {
		Message string
		Height  int
	}

	cdc := amino.NewCodec()
	cdc.RegisterInterface((*message)(nil), nil)
	cdc.RegisterConcrete(&simpleMsg{}, name, nil)

	bm := &simpleMsg{Message: "ABC", Height: 100}
	pbm := &p3.SimpleMsg{Message: "ABC", Height: 100}
	bmUnreg := &simpleMsgUnregistered{bm.Message, bm.Height}

	bz, err := cdc.MarshalBinaryBare(bm)
	require.NoError(t, err)

	bzUnreg, err := cdc.MarshalBinaryBare(bmUnreg)
	require.NoError(t, err)

	// encoded bytes decodeable via protobuf:
	pAny := &p3.AminoRegisteredAny{}
	err = proto.Unmarshal(bz, pAny)
	require.NoError(t, err)

	// amino encoded value / prefix matches proto encoding
	assert.Equal(t, pAny.Value, bzUnreg)
	_, prefix := amino.NameToDisfix(name)
	assert.Equal(t, pAny.AminoPreOrDisfix, prefix.Bytes())
	pbz, err := proto.Marshal(pbm)
	require.NoError(t, err)
	assert.Equal(t, pbz, bzUnreg)
}

func TestDisambExample(t *testing.T) {
	const name = "interfaceFields"
	cdc := amino.NewCodec()
	cdc.RegisterInterface((*tests.Interface1)(nil), &amino.InterfaceOptions{
		AlwaysDisambiguate: true,
	})
	cdc.RegisterConcrete((*tests.InterfaceFieldsStruct)(nil), name, nil)

	i1 := &tests.InterfaceFieldsStruct{F1: new(tests.InterfaceFieldsStruct), F2: nil}
	bz, err := cdc.MarshalBinaryBare(i1)
	type fieldStructUnreg struct {
		F1 tests.Interface1
	}

	concrete1 := &fieldStructUnreg{F1: new(tests.InterfaceFieldsStruct)}
	bc, err := cdc.MarshalBinaryBare(concrete1)
	require.NoError(t, err)
	t.Logf("%#v", bz)

	pAny := &p3.AminoRegisteredAny{}
	err = proto.Unmarshal(bz, pAny)
	require.NoError(t, err)

	disamb, prefix := amino.NameToDisfix(name)
	t.Logf("%#v", disamb[:])
	//
	//t.Logf("%v",disfix[:])
	// TODO: apparently the disamb bytes are only used for the fields
	//  and not for the outer interface. Not sure why this makes sense.
	// assert.Equal(t, disfix, pAny.AminoPreOrDisfix)
	assert.Equal(t, prefix.Bytes(), pAny.AminoPreOrDisfix)
	assert.Equal(t, bc, pAny.Value)

	embeddedAny := &p3.EmbeddedRegisteredAny{}
	err = proto.Unmarshal(pAny.Value, embeddedAny)
	t.Logf("embeddedAny = %#v", embeddedAny)

	require.NoError(t, err)
	pAnyInner := &p3.AminoRegisteredAny{}

	t.Logf("pAny.Value = %#v", pAny.Value)
	err = proto.Unmarshal(pAny.Value, pAnyInner)
	require.NoError(t, err)

	//disfix := bytes.Join([][]byte{disamb[:], prefix[:]}, []byte(""))
	//assert.Equal(t, disfix, pAnyInner.AminoPreOrDisfix)
	t.Logf("aminoInner %#v", pAnyInner.AminoPreOrDisfix)

	aminoAnyInner := &amino.RegisteredAny{}
	err = cdc.UnmarshalBinaryBare(pAny.Value, aminoAnyInner)
	require.NoError(t, err)
	//assert.Equal(t, disfix, aminoAnyInner.AminoPreOrDisfix)

	i2 := new(tests.InterfaceFieldsStruct)
	err = cdc.UnmarshalBinaryBare(bz, i2)
	require.NoError(t, err)
	require.Equal(t, i1, i2, "i1 and i2 should be the same after decoding")
}
