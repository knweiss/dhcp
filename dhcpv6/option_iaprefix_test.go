package dhcpv6

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptIAPrefix(t *testing.T) {
	buf := []byte{
		0xaa, 0xbb, 0xcc, 0xdd,                         // preferredLifetime
		0xee, 0xff, 0x00, 0x11,                         // validLifetime
		36,                                             // prefixLength
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, // ipv6Prefix
	}
	opt, err := ParseOptIAPrefix(buf)
	if err != nil {
		t.Fatal(err)
	}
	if pl := opt.PreferredLifetime(); pl != 0xaabbccdd {
		t.Fatalf("Invalid Preferred Lifetime. Expected 0xaabbccdd, got %v", pl)
	}
	if vl := opt.ValidLifetime(); vl != 0xeeff0011 {
		t.Fatalf("Invalid Valid Lifetime. Expected 0xeeff0011, got %v", vl)
	}
	if pr := opt.PrefixLength(); pr != 36 {
		t.Fatalf("Invalid Prefix Length. Expected 36, got %v", pr)
	}
	if ip := opt.IPv6Prefix(); !bytes.Equal(ip, net.IPv6loopback) {
		t.Fatalf("Invalid Prefix Length. Expected %v, got %v", net.IPv6loopback, ip)
	}
}

func TestOptIAPrefixToBytes(t *testing.T) {
	buf := []byte{
		0xaa, 0xbb, 0xcc, 0xdd,                         // preferredLifetime
		0xee, 0xff, 0x00, 0x11,                         // validLifetime
		36,                                             // prefixLength
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // ipv6Prefix
	}
	expected := []byte{00, 26, 00, byte(len(buf))}
	expected = append(expected, buf...)
	opt := OptIAPrefix{
		preferredLifetime: 0xaabbccdd,
		validLifetime: 0xeeff0011,
		prefixLength: 36,
	}
	toBytes := opt.ToBytes()
	if !bytes.Equal(toBytes, expected) {
		t.Fatalf("Invalid ToBytes result. Expected %v, got %v", expected, toBytes)
	}
}

func TestOptIAPrefixParseInvalidTooShort(t *testing.T) {
	buf := []byte{
		0xaa, 0xbb, 0xcc, 0xdd, // preferredLifetime
		0xee, 0xff, 0x00, 0x11, // validLifetime
		36,                     // prefixLength
		0, 0, 0, 0, 0, 0, 0,    // truncated ipv6Prefix
	}
	if opt, err := ParseOptIAPrefix(buf); err == nil {
		t.Fatalf("ParseOptIAPrefix: Expected error on truncated option, got %v", opt)
	}
}

func TestOptIAPrefixSetGetOptions(t *testing.T) {
	opt := OptIAPrefix{
		preferredLifetime: 0xaabbccdd,
		validLifetime: 0xeeff0011,
	}
	expected := []byte{
		0, 8, 0, 2, 0xaa, 0xbb, // options
	}

	require.Equal(t, []byte(nil), opt.Options(), "Options() should be blank by default")
	opt.SetOptions(expected)
	require.Equal(t, expected, opt.Options(), "Options() did not contain the correct data")
}

func TestOptIAPrefixString(t *testing.T) {
	buf := []byte{
		0xaa, 0xbb, 0xcc, 0xdd,                         // preferredLifetime
		0xee, 0xff, 0x00, 0x11,                         // validLifetime
		36,                                             // prefixLength
		0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // ipv6Prefix
	}
	opt, err := ParseOptIAPrefix(buf)
	require.NoError(t, err)

	str := opt.String()
	require.Contains(
		t, str,
		"ipv6prefix=2001:db8::",
		"String() should return the ipv6addr",
	)
	require.Contains(
		t, str,
		"preferredlifetime=2864434397",
		"String() should return the preferredlifetime",
	)
	require.Contains(
		t, str,
		"validlifetime=4009689105",
		"String() should return the validlifetime",
	)
}
