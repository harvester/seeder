package util

import (
	"fmt"
	"time"

	"github.com/rancher/wrangler/pkg/condition"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

// ConditionExists checks if a condition exists.
func ConditionExists(i *seederv1alpha1.Inventory, cond condition.Cond) bool {
	return cond.IsTrue(i)
}

// RemoveCondition removes the named condition
func RemoveCondition(i *seederv1alpha1.Inventory, cond condition.Cond) {
	cond.False(i)
	cond.Message(i, "")
	cond.Reason(i, "")
}

// CreateOrUpdateCondition creates or updates the status of an existing condition
func CreateOrUpdateCondition(i *seederv1alpha1.Inventory, cond condition.Cond, message string) {
	cond.SetStatus(i, message)
	cond.True(i)
}

func SetErrorCondition(i *seederv1alpha1.Inventory, cond condition.Cond, message string) {
	now := time.Now().UTC().Format(time.RFC3339)
	cond.SetError(i, "", fmt.Errorf(message))
	cond.True(i)
	cond.LastUpdated(i, now)
}
