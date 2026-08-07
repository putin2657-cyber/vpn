package billing

import "testing"

func TestOwnerPeersAreBlockedFromInvite(t *testing.T) {
	for _, name := range []string{"my-phone", "my-second", "device-03", "device-06"} {
		if !IsOwnerPeer(name) {
			t.Fatalf("%s should be an owner peer", name)
		}
	}
	if IsOwnerPeer("device-04") {
		t.Fatal("device-04 should be available for family billing")
	}

	svc := &Service{}
	if _, _, err := svc.Invite("Mama", "my-phone"); err == nil {
		t.Fatal("expected invite to owner peer to fail")
	}
}
