package outreach

import (
	"errors"
	"fmt"
	"net/textproto"
	"testing"
)

func TestClassifySendError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		wantSt Status
		want   string
	}{
		{"permanent 550 bounce", &textproto.Error{Code: 550, Msg: "user unknown"}, StatusBounced, "permanent rejection (550)"},
		{"permanent 554 bounce", &textproto.Error{Code: 554, Msg: "relay denied"}, StatusBounced, "permanent rejection (554)"},
		{"transient 450 stays failed", &textproto.Error{Code: 450, Msg: "try later"}, StatusFailed, "transient failure (450)"},
		{"transient 421 stays failed", &textproto.Error{Code: 421, Msg: "too busy"}, StatusFailed, "transient failure (421)"},
		{"wrapped permanent still bounced", fmt.Errorf("relay send: %w", &textproto.Error{Code: 550, Msg: "no such user"}), StatusBounced, "permanent rejection (550)"},
		{"wrapped transient still failed", fmt.Errorf("smtp send: %w", &textproto.Error{Code: 452, Msg: "full"}), StatusFailed, "transient failure (452)"},
		{"plain network error failed", errors.New("dial tcp: timeout"), StatusFailed, "send error"},
		{"nil error never classifies", nil, StatusFailed, "send error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, why := classifySendError(tt.err)
			if st != tt.wantSt || why != tt.want {
				t.Errorf("classifySendError(%v) = (%s, %q); want (%s, %q)", tt.err, st, why, tt.wantSt, tt.want)
			}
		})
	}
}
