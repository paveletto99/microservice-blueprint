package client

import "testing"

func TestPaymentClientRequiresLoopback(t *testing.T) {
	for _, address := range []string{"payment:50051", "0.0.0.0:50051", "10.0.0.1:50051", "127.0.0.1:0", "http://127.0.0.1:50051"} {
		if c, err := NewPaymentClient(address); err == nil {
			c.Close()
			t.Errorf("accepted %q", address)
		}
	}
	c, err := NewPaymentClient("127.0.0.1:50051")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}
