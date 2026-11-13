// Tests for converting binary SQL Server column values to readable text.
package sqldb

import "testing"

func TestBinaryColumnValueGUID(t *testing.T) {
	// Raw bytes of GUID 6F9619FF-8B86-D011-B42D-00C04FC964FF as SQL Server sends them:
	// the first three groups are stored in reverse byte order.
	rawBytes := []byte{0xFF, 0x19, 0x96, 0x6F, 0x86, 0x8B, 0x11, 0xD0, 0xB4, 0x2D, 0x00, 0xC0, 0x4F, 0xC9, 0x64, 0xFF}
	got := binaryColumnValue(rawBytes, "UNIQUEIDENTIFIER")
	if got != "6F9619FF-8B86-D011-B42D-00C04FC964FF" {
		t.Errorf("GUID = %v, want 6F9619FF-8B86-D011-B42D-00C04FC964FF", got)
	}
}

func TestBinaryColumnValueVarbinary(t *testing.T) {
	got := binaryColumnValue([]byte{0x01, 0xAB}, "VARBINARY")
	if got != "0x01AB" {
		t.Errorf("VARBINARY = %v, want 0x01AB", got)
	}
}

func TestBinaryColumnValueLeavesTextLikeBytes(t *testing.T) {
	got, isBytes := binaryColumnValue([]byte("123.45"), "DECIMAL").([]byte)
	if !isBytes || string(got) != "123.45" {
		t.Errorf("DECIMAL bytes should be returned unchanged, got %v", got)
	}
}
