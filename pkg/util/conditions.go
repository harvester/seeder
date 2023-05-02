package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

// ConditionExists checks if a condition exists.
func ConditionExists(conditions []seederv1alpha1.Conditions, t seederv1alpha1.ConditionType) bool {
	for _, v := range conditions {
		if v.Type == t {
			return true
		}
	}
	return false
}

//RemoveCondition removes the named condition
func RemoveCondition(conditions []seederv1alpha1.Conditions, t seederv1alpha1.ConditionType) []seederv1alpha1.Conditions {
	var retConditions []seederv1alpha1.Conditions
	for _, v := range conditions {
		if v.Type != t {
			retConditions = append(retConditions, v)
		}
	}
	return retConditions
}

// CreateOrUpdateCondition creates or updates the status of an existing condition
func CreateOrUpdateCondition(conditions []seederv1alpha1.Conditions, t seederv1alpha1.ConditionType, message string) []seederv1alpha1.Conditions {
	var newConditions []seederv1alpha1.Conditions
	if ConditionExists(conditions, t) {
		for _, v := range conditions {
			if v.Type == t {
				v.LastUpdateTime = metav1.Now()
				v.Message = message
			}
			newConditions = append(newConditions, v)
		}

	} else {
		newConditions = append(newConditions, seederv1alpha1.Conditions{
			Type:      t,
			Message:   message,
			StartTime: metav1.Now(),
		})
		newConditions = append(newConditions, conditions...)
	}
	return newConditions
}
