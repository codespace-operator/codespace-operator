package helpers

import (
	"k8s.io/client-go/util/retry"
)

// retryOnConflict runs fn with standard backoff if a 409 occurs.
func RetryOnConflict(fn func() error) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return fn()
	})
}
