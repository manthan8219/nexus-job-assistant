package outreach

import (
	"errors"
	"fmt"
	"net/textproto"
)

// classifySendError maps an SMTP send failure to a terminal item status.
// Permanent (5xx) rejections from the recipient server are a hard bounce — the
// address is bad and retrying will never succeed, so the sequence stops at
// StatusBounced. Transient (4xx) errors keep the item retryable as StatusFailed.
// Non-SMTP errors (auth, network, API) are also failed, never bounced.
func classifySendError(err error) (Status, string) {
	var proto *textproto.Error
	if errors.As(err, &proto) {
		switch {
		case proto.Code >= 500 && proto.Code < 600:
			return StatusBounced, fmt.Sprintf("permanent rejection (%d)", proto.Code)
		case proto.Code >= 400 && proto.Code < 500:
			return StatusFailed, fmt.Sprintf("transient failure (%d)", proto.Code)
		}
	}
	return StatusFailed, "send error"
}
