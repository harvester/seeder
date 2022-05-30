package util

import (
	"testing"

	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testConditionData = []bmaasv1alpha1.Conditions{
		{
			Type:      bmaasv1alpha1.BMCTaskRequest,
			StartTime: metav1.Now(),
			Message:   "BMC Request submitted",
		},
	}
)

func Test_ConditionsExist(t *testing.T) {
	ok := ConditionExists(testConditionData, bmaasv1alpha1.BMCTaskRequest)
	assert.True(t, ok, "expected condition to be found")
}

func Test_ConditionsExist_False(t *testing.T) {
	ok := ConditionExists(testConditionData, bmaasv1alpha1.BMCTaskComplete)
	assert.False(t, ok, "expected condition to be not found")
}

func Test_RemoveCondition(t *testing.T) {
	newConditions := RemoveCondition(testConditionData, bmaasv1alpha1.BMCTaskRequest)
	ok := ConditionExists(newConditions, bmaasv1alpha1.BMCTaskRequest)
	assert.False(t, ok, "expected condition to be not found")
}

func Test_AddCondition(t *testing.T) {
	newConditions := CreateOrUpdateCondition(testConditionData, bmaasv1alpha1.BMCTaskComplete, "task completed")
	ok := ConditionExists(newConditions, bmaasv1alpha1.BMCTaskComplete)
	assert.True(t, ok, "expected new condition to be present")
}

func Test_UpdateCondition(t *testing.T) {
	orgTime := testConditionData[0].StartTime
	newConditions := CreateOrUpdateCondition(testConditionData, bmaasv1alpha1.BMCTaskRequest, "new task request")
	ok := ConditionExists(newConditions, bmaasv1alpha1.BMCTaskRequest)
	assert.True(t, ok, "expected condition to be present")
	assert.Equal(t, orgTime, newConditions[0].StartTime, "original time should be unchanged")
	assert.NotEmpty(t, newConditions[0].LastUpdateTime, "lastUpdateTime should not be empty")
}
