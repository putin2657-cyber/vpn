package awg

import "testing"

func TestParseShowDump(t *testing.T) {
	raw := []byte("uOE6EPckkocLT+W67bloYyggdVeFQJGwFYoqxJMAuVo=\tIW+r70r0f0EjijRzH1HWwITH7uwy2NUuIxIXE0wlt2k=\t51820\t4\t8\t80\t33\t139\t0\t0\t275666844\t1875830948\t30171185\t1538686353\t(null)\t(null)\t(none)\t0\t0\t0\t0\t0\t0\toff\n" +
		"KH68a37sTf2iKov/dHnVX8B7NGzPeBqwQ5rmP5PPqVQ=\tpsk\t1.2.3.4:1234\t10.8.0.3/32\t1710000000\t1000\t2000\toff\n" +
		"deadbeefAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\tpsk\t(none)\t10.8.0.2/32\t0\t0\t0\toff\n")
	peers, err := parseShowDump(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("want 2 peers, got %d", len(peers))
	}
	p, ok := peers["KH68a37sTf2iKov/dHnVX8B7NGzPeBqwQ5rmP5PPqVQ="]
	if !ok {
		t.Fatal("missing peer")
	}
	if p.Endpoint != "1.2.3.4:1234" || p.RxBytes != 1000 || p.TxBytes != 2000 {
		t.Fatalf("unexpected peer: %+v", p)
	}
	if peers["deadbeefAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="].HandshakeHuman != "never" {
		t.Fatalf("want never, got %q", peers["deadbeefAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="].HandshakeHuman)
	}
}

func TestFormatBytes(t *testing.T) {
	if FormatBytes(2048) != "2.0 KiB" {
		t.Fatalf("got %q", FormatBytes(2048))
	}
}
