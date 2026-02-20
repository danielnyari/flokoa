package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// updateStatusWithRetry performs a status update with automatic retry on conflict.
// On conflict, it re-fetches the resource to get the latest ResourceVersion, calls
// applyStatus to reapply the desired status fields, then retries the update.
// This prevents silent status update failures under concurrent reconciliation (fixes #95).
func updateStatusWithRetry(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	applyStatus func(),
) error {
	applyStatus()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := c.Status().Update(ctx, obj)
		if !apierrors.IsConflict(err) {
			return err
		}
		// Re-fetch to get latest ResourceVersion
		if getErr := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); getErr != nil {
			return getErr
		}
		// Reapply the desired status fields
		applyStatus()
		return c.Status().Update(ctx, obj)
	})
}
