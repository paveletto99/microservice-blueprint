/*===========================================================================*\

\*===========================================================================*/

package service

import "testing"

func TestPoboRunner(t *testing.T) {
	x := NewPobo()
	if x == nil {
		t.Errorf("Failure")
	}
}
