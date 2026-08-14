package collector

import (
	"testing"
)

func TestParseNetDevData(t *testing.T) {
	sample := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 12345678       0    0    0    0     0          0         0 12345678       0    0    0    0     0       0          0
  eth0: 10000000     500    0    0    0     0          0         0 20000000     600    0    0    0     0       0          0
docker0: 9999999       0    0    0    0     0          0         0  9999999       0    0    0    0     0       0          0
  ens3:  5000000     200    0    0    0     0          0         0  8000000     300    0    0    0     0       0          0
 veth1: 11111111       0    0    0    0     0          0         0 11111111       0    0    0    0     0       0          0
`
	rx, tx := parseNetDevData(sample)
	expectedRx := uint64(10000000 + 5000000)
	expectedTx := uint64(20000000 + 8000000)

	if rx != expectedRx {
		t.Errorf("expected rx %d, got %d", expectedRx, rx)
	}
	if tx != expectedTx {
		t.Errorf("expected tx %d, got %d", expectedTx, tx)
	}
}
