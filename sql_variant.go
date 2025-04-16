package mssql

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/microsoft/go-mssqldb/internal/cp"
	"github.com/microsoft/go-mssqldb/msdsn"
)

// sql_variant data type
// https://learn.microsoft.com/sql/t-sql/data-types/sql-variant-transact-sql?view=sql-server-ver16
const (
	SQLVariantTypeGuid            = typeGuid
	SQLVariantTypeBit             = typeBit
	SQLVariantTypeInt1            = typeInt1
	SQLVariantTypeInt2            = typeInt2
	SQLVariantTypeInt4            = typeInt4
	SQLVariantTypeInt8            = typeInt8
	SQLVariantTypeDateTime        = typeDateTime
	SQLVariantTypeDateTim4        = typeDateTim4
	SQLVariantTypeFlt4            = typeFlt4
	SQLVariantTypeFlt8            = typeFlt8
	SQLVariantTypeMoney4          = typeMoney4
	SQLVariantTypeMoney           = typeMoney
	SQLVariantTypeDateN           = typeDateN
	SQLVariantTypeTimeN           = typeTimeN
	SQLVariantTypeDateTime2N      = typeDateTime2N
	SQLVariantTypeDateTimeOffsetN = typeDateTimeOffsetN
	SQLVariantTypeBigVarBin       = typeBigVarBin
	SQLVariantTypeBigBinary       = typeBigBinary
	SQLVariantTypeDecimalN        = typeDecimalN
	SQLVariantTypeNumericN        = typeNumericN
	SQLVariantTypeBigVarChar      = typeBigVarChar
	SQLVariantTypeBigChar         = typeBigChar
	SQLVariantTypeNVarChar        = typeNVarChar
	SQLVariantTypeNChar           = typeNChar
)

// NullSQLVariant represents a nullable SQLVariant type,
// containing a SQLVariant value and a validity flag.
type NullSQLVariant struct {
	SQLVariant SQLVariant
	Valid      bool
}

func (v *NullSQLVariant) Scan(val any) error {
	switch vt := val.(type) {
	case []byte:
		v.decodeVariant(vt)
		return nil
	case nil:
		v.Valid = false
		return nil
	default:
		return fmt.Errorf("mssql: invalid type for variant column: %T", val)
	}
}

func (v *NullSQLVariant) encode() ([]byte, error) {
	if v.Valid {
		return v.SQLVariant.encode()
	}
	return nil, nil
}

// SQLVariant represents a variant type in SQL Server, capable of storing various data types dynamically.
// https://learn.microsoft.com/openspecs/windows_protocols/ms-tds/2435e85d-9e61-492c-acb2-627ffccb5b92
type SQLVariant struct {
	BaseType  uint8
	Scale     uint8
	Precision uint8
	MaxLength uint16
	Collation SQLVariantCollation
	Value     any
}

// SQLVariantCollation represents the collation information for a SQL Server variant type.
// https://learn.microsoft.com/openspecs/windows_protocols/ms-tds/3d29e8dc-218a-42c6-9ba4-947ebca9fd7e
type SQLVariantCollation struct {
	LcidAndFlags uint32
	SortId       uint8
}

func (v *SQLVariant) Scan(val any) error {
	switch vt := val.(type) {
	case []byte:
		var nullVariant NullSQLVariant
		nullVariant.decodeVariant(vt)
		if nullVariant.Valid == false {
			return fmt.Errorf("mssql: variant is null")
		}
		*v = nullVariant.SQLVariant
		return nil
	default:
		return fmt.Errorf("mssql: invalid type for variant column: %T", val)
	}
}

// encodes sql_variant type
// https://learn.microsoft.com/openspecs/windows_protocols/ms-tds/2435e85d-9e61-492c-acb2-627ffccb5b92
func (v *SQLVariant) encode() ([]byte, error) {
	switch v.BaseType {
	case typeGuid:
		return v.encodeGuid()
	case typeBit:
		return v.encodeBit()
	case typeInt1:
		return v.encodeInt(1)
	case typeInt2:
		return v.encodeInt(2)
	case typeInt4:
		return v.encodeInt(4)
	case typeInt8:
		return v.encodeInt(8)
	case typeDateTime:
		return v.encodeDateTime()
	case typeDateTim4:
		return v.encodeDateTim4()
	case typeFlt4:
		return v.encodeFloat(4)
	case typeFlt8:
		return v.encodeFloat(8)
	case typeMoney4:
		return v.encodeMoney4()
	case typeMoney:
		return v.encodeMoney()
	case typeDateN:
		return v.encodeDateN()
	case typeTimeN:
		return v.encodeTimeN()
	case typeDateTime2N:
		return v.encodeDateTime2()
	case typeDateTimeOffsetN:
		return v.encodeDateTimeOffset()
	case typeBigVarBin, typeBigBinary:
		return v.encodeBinary()
	case typeDecimalN, typeNumericN:
		return v.encodeDecimal()
	case typeBigVarChar, typeBigChar:
		return v.encodeBigChar()
	case typeNVarChar, typeNChar:
		return v.encodeNChar()
	default:
		badStreamPanicf("Invalid variant typeid")
		return nil, nil
	}
}

func (v *SQLVariant) encodeGuid() ([]byte, error) {
	var guid []byte
	switch v := v.Value.(type) {
	case []byte:
		guid = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for guid column: %T", v)
	}

	// GuidConversion is always true because on decode we always make conversation of guid.
	valueBytes := encodeGuid(guid, msdsn.EncodeParameters{GuidConversion: true})
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeBit() ([]byte, error) {
	var bit bool
	switch v := v.Value.(type) {
	case bool:
		bit = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for bit column: %T", v)
	}

	if bit {
		return v.encodeZeroProp([]byte{1}), nil
	}
	return v.encodeZeroProp([]byte{0}), nil
}

func (v *SQLVariant) encodeInt(size int) ([]byte, error) {
	valueBytes, err := encodeIntValue(v.Value, size)
	if err != nil {
		return nil, err
	}
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeDateTime() ([]byte, error) {
	var t time.Time
	switch v := v.Value.(type) {
	case time.Time:
		t = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for time column: %T", v)
	}

	valueBytes := encodeDateTime(t)
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeDateTim4() ([]byte, error) {
	var t time.Time
	switch v := v.Value.(type) {
	case time.Time:
		t = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for time column: %T", v)
	}

	valueBytes := encodeDateTim4(t)
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeFloat(size int) ([]byte, error) {
	valueBytes, err := encodeFloatValue(v.Value, size)
	if err != nil {
		return nil, err
	}
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeMoney4() ([]byte, error) {
	var money []byte
	switch v := v.Value.(type) {
	case []byte:
		money = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for money column: %T", v)
	}

	valueBytes, err := encodeMoney4(money)
	if err != nil {
		return nil, err
	}
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeMoney() ([]byte, error) {
	var money []byte
	switch v := v.Value.(type) {
	case []byte:
		money = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for money column: %T", v)
	}

	valueBytes, err := encodeMoney(money)
	if err != nil {
		return nil, err
	}
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeDateN() ([]byte, error) {
	var t time.Time
	switch v := v.Value.(type) {
	case time.Time:
		t = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for time column: %T", v)
	}

	valueBytes := encodeDate(t)
	return v.encodeZeroProp(valueBytes), nil
}

func (v *SQLVariant) encodeTimeN() ([]byte, error) {
	var t time.Time
	switch v := v.Value.(type) {
	case time.Time:
		t = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for time column: %T", v)
	}

	valueBytes := encodeTime(t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), int(v.Scale))
	return v.encodeWithScale(valueBytes), nil
}

func (v *SQLVariant) encodeDateTime2() ([]byte, error) {
	var t time.Time
	switch v := v.Value.(type) {
	case time.Time:
		t = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for time column: %T", v)
	}

	valueBytes := encodeDateTime2(t, int(v.Scale))
	return v.encodeWithScale(valueBytes), nil
}

func (v *SQLVariant) encodeDateTimeOffset() ([]byte, error) {
	var t time.Time
	switch v := v.Value.(type) {
	case time.Time:
		t = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for time column: %T", v)
	}

	valueBytes := encodeDateTimeOffset(t, int(v.Scale))
	return v.encodeWithScale(valueBytes), nil
}

func (v *SQLVariant) encodeBinary() ([]byte, error) {
	var b []byte
	switch v := v.Value.(type) {
	case []byte:
		b = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for binary column: %T", v)
	}
	return v.encodeWithMaxLength(b), nil
}

func (v *SQLVariant) encodeDecimal() ([]byte, error) {
	valueBytes, err := encodeDecimal(v.Value, v.Precision, v.Scale)
	if err != nil {
		return nil, err
	}
	return v.encodeWithPrecisionAndScale(valueBytes), nil
}

func (v *SQLVariant) encodeBigChar() ([]byte, error) {
	var s string
	switch v := v.Value.(type) {
	case string:
		s = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for string column: %T", v)
	}
	return v.encodeWithCollationAndMaxLength([]byte(s)), nil
}

func (v *SQLVariant) encodeNChar() ([]byte, error) {
	var s string
	switch v := v.Value.(type) {
	case string:
		s = v
	default:
		return nil, fmt.Errorf("mssql: invalid type for string column: %T", v)
	}
	return v.encodeWithCollationAndMaxLength(str2ucs2(s)), nil
}

func (v *SQLVariant) encodeZeroProp(val []byte) []byte {
	res := make([]byte, len(val)+2)
	res[0] = v.BaseType
	res[1] = byte(0)
	copy(res[2:], val)
	return res
}

func (v *SQLVariant) encodeWithScale(val []byte) []byte {
	res := make([]byte, len(val)+3)
	res[0] = v.BaseType
	res[1] = byte(1)
	res[2] = v.Scale
	copy(res[3:], val)
	return res
}

func (v *SQLVariant) encodeWithMaxLength(val []byte) []byte {
	res := make([]byte, len(val)+4)
	res[0] = v.BaseType
	res[1] = byte(2)
	binary.LittleEndian.PutUint16(res[2:], v.MaxLength)
	copy(res[4:], val)
	return res
}

func (v *SQLVariant) encodeWithPrecisionAndScale(val []byte) []byte {
	res := make([]byte, len(val)+4)
	res[0] = v.BaseType
	res[1] = byte(2)
	res[2] = v.Precision
	res[3] = v.Scale
	copy(res[4:], val)
	return res
}

func (v *SQLVariant) encodeWithCollationAndMaxLength(val []byte) []byte {
	res := make([]byte, len(val)+9)
	res[0] = v.BaseType
	res[1] = byte(7)
	binary.LittleEndian.PutUint32(res[2:], v.Collation.LcidAndFlags)
	res[6] = v.Collation.SortId
	binary.LittleEndian.PutUint16(res[7:], v.MaxLength)
	copy(res[9:], val)
	return res
}

type variantBuffer struct {
	rbuf *bytes.Reader
}

func (r *variantBuffer) byte() byte {
	b, err := r.rbuf.ReadByte()
	if err != nil {
		badStreamPanic(err)
	}
	return b
}

func (r *variantBuffer) uint16() uint16 {
	var res uint16
	err := binary.Read(r.rbuf, binary.LittleEndian, &res)
	if err != nil {
		badStreamPanic(err)
	}
	return res
}

func (r *variantBuffer) uint32() uint32 {
	var buf [4]byte
	r.rbuf.Read(buf[:])
	return uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
}

func (r *variantBuffer) uint64() uint64 {
	var res uint64
	err := binary.Read(r.rbuf, binary.LittleEndian, &res)
	if err != nil {
		badStreamPanic(err)
	}
	return res
}

func (r *variantBuffer) readCollation() (res SQLVariantCollation) {
	res.LcidAndFlags = r.uint32()
	res.SortId = r.byte()
	return
}

func (r *variantBuffer) readValue(size uint32) []byte {
	buf := make([]byte, size)
	_, err := io.ReadFull(r.rbuf, buf)
	if err != nil {
		badStreamPanic(err)
	}
	return buf
}

// decodes variant value
// https://learn.microsoft.com/openspecs/windows_protocols/ms-tds/2435e85d-9e61-492c-acb2-627ffccb5b92
func (v *NullSQLVariant) decodeVariant(buf []byte) {
	r := variantBuffer{rbuf: bytes.NewReader(buf)}

	size := len(buf)
	if size == 0 {
		v.Valid = false
		return
	}

	vartype := r.byte()
	propbytes := r.byte()
	valueSize := uint32(size) - 2 - uint32(propbytes)

	sqlVariant := SQLVariant{
		BaseType: vartype,
	}
	switch vartype {
	case typeGuid:
		// GuidConversion is always true to ensure GUIDs are decoded with correct endianness.
		// decodeGuid reverses bytes internally to produce standard format.
		sqlVariant.Value = decodeGuid(r.readValue(valueSize), msdsn.EncodeParameters{GuidConversion: true})
	case typeBit:
		sqlVariant.Value = r.byte() != 0
	case typeInt1:
		sqlVariant.Value = int64(r.byte())
	case typeInt2:
		sqlVariant.Value = int64(int16(r.uint16()))
	case typeInt4:
		sqlVariant.Value = int64(r.uint32())
	case typeInt8:
		sqlVariant.Value = int64(r.uint64())
	case typeDateTime:
		sqlVariant.Value = decodeDateTime(r.readValue(valueSize))
	case typeDateTim4:
		sqlVariant.Value = decodeDateTim4(r.readValue(valueSize))
	case typeFlt4:
		sqlVariant.Value = float64(math.Float32frombits(r.uint32()))
	case typeFlt8:
		sqlVariant.Value = math.Float64frombits(r.uint64())
	case typeMoney4:
		sqlVariant.Value = decodeMoney4(r.readValue(valueSize))
	case typeMoney:
		sqlVariant.Value = decodeMoney(r.readValue(valueSize))
	case typeDateN:
		sqlVariant.Value = decodeDate(r.readValue(valueSize))
	case typeTimeN:
		scale := r.byte()
		sqlVariant.Scale = scale
		sqlVariant.Value = decodeTime(scale, r.readValue(valueSize))
	case typeDateTime2N:
		scale := r.byte()
		sqlVariant.Scale = scale
		sqlVariant.Value = decodeDateTime2(scale, r.readValue(valueSize))
	case typeDateTimeOffsetN:
		scale := r.byte()
		sqlVariant.Scale = scale
		sqlVariant.Value = decodeDateTimeOffset(scale, r.readValue(valueSize))
	case typeBigVarBin, typeBigBinary:
		sqlVariant.MaxLength = r.uint16()
		sqlVariant.Value = r.readValue(valueSize)
	case typeDecimalN, typeNumericN:
		prec := r.byte()
		scale := r.byte()
		sqlVariant.Scale = scale
		sqlVariant.Precision = prec
		sqlVariant.Value = decodeDecimal(prec, scale, r.readValue(valueSize))
	case typeBigVarChar, typeBigChar:
		sqlVariant.Collation = r.readCollation()
		sqlVariant.MaxLength = r.uint16()
		col := cp.Collation{
			LcidAndFlags: sqlVariant.Collation.LcidAndFlags,
			SortId:       sqlVariant.Collation.SortId,
		}
		sqlVariant.Value = decodeChar(col, r.readValue(valueSize))
	case typeNVarChar, typeNChar:
		sqlVariant.Collation = r.readCollation()
		sqlVariant.MaxLength = r.uint16()
		sqlVariant.Value = decodeNChar(r.readValue(valueSize))
	default:
		badStreamPanicf("Invalid variant typeid")
	}

	v.SQLVariant = sqlVariant
	v.Valid = true
	return
}
