package util

import (
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionExists checks if a condition exists.
func ConditionExists(conditions []bmaasv1alpha1.Conditions, t bmaasv1alpha1.ConditionType) bool {
	for _, v := range conditions {
		if v.Type == t {
			return true
		}
	}
	return false
}

//RemoveCondition removes the named condition
func RemoveCondition(conditions []bmaasv1alpha1.Conditions, t bmaasv1alpha1.ConditionType) []bmaasv1alpha1.Conditions {
	var retConditions []bmaasv1alpha1.Conditions
	for _, v := range conditions {
		if v.Type != t {
			retConditions = append(retConditions, v)
		}
	}
	return retConditions
}

// CreateOrUpdateCondition creates or updates the status of an existing condition
func CreateOrUpdateCondition(conditions []bmaasv1alpha1.Conditions, t bmaasv1alpha1.ConditionType, message string) []bmaasv1alpha1.Conditions {
	var newConditions []bmaasv1alpha1.Conditions
	if ConditionExists(conditions, t) {
		for _, v := range conditions {
			if v.Type == t {
				v.LastUpdateTime = metav1.Now()
				v.Message = message
			}
			newConditions = append(newConditions, v)
		}
	} else {
		newConditions = append(newConditions, bmaasv1alpha1.Conditions{Type: t,
			Message:   message,
			StartTime: metav1.Now(),
		})
	}
	return newConditions
}
