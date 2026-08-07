package db

import (
	"testing"
	"time"
)

func TestCreateMemberAndExtendPaidUntil(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatal(err)
	}
	peer, err := database.AddPeer("device-03")
	if err != nil {
		t.Fatal(err)
	}

	member, err := database.CreateMember("Mama", peer.Name)
	if err != nil {
		t.Fatal(err)
	}
	if member.Token == "" || member.Status != MemberInvited {
		t.Fatalf("unexpected member: %+v", member)
	}

	byToken, err := database.GetMemberByToken(member.Token)
	if err != nil || byToken.ID != member.ID {
		t.Fatalf("GetMemberByToken: %v %+v", err, byToken)
	}

	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	first := ExtendPaidUntil("", now, 30*24*time.Hour)
	second := ExtendPaidUntil(first, now, 30*24*time.Hour)
	if first == second {
		t.Fatal("second extension should move paid_until forward")
	}
}

func TestDisabledPeersOmittedFromAWGPeers(t *testing.T) {
	database := openTestDB(t)
	if err := database.SaveServer(testServer(t, "10.8.0.0/24")); err != nil {
		t.Fatal(err)
	}
	if _, err := database.AddPeer("paid"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.AddPeer("unpaid"); err != nil {
		t.Fatal(err)
	}
	if err := database.SetPeerEnabled("unpaid", false); err != nil {
		t.Fatal(err)
	}

	peers, err := database.AWGPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].Name != "paid" {
		t.Fatalf("got %+v", peers)
	}
}
