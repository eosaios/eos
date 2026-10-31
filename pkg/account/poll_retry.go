package account

import (
	"errors"
	"time"
)

// Match Todo's flow contract: a temporary network failure or rate limit is
// waiting, not a terminal sign-in failure. Ticket exchange is never replayed.
func retryableFlowPoll(err error) bool {
	var apiErr *apiError
	return errors.Is(err, errNetwork) || (errors.As(err, &apiErr) && apiErr.Status == 429)
}

func retryableProfileSync(err error) bool {
	var apiErr *apiError
	return retryableFlowPoll(err) || (errors.As(err, &apiErr) && apiErr.Status >= 500)
}

func flowPollDelay(f *flow, err error) time.Duration {
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		f.rateStrikes = 0
		return 2 * time.Second
	}
	f.rateStrikes++
	return time.Duration(1<<min(f.rateStrikes, 3)) * time.Second
}
