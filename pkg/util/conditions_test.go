package util

import (
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testConditionData = []seederv1alpha1.Conditions{
		{
			Type:      seederv1alpha1.BMCObjectCreated,
			StartTime: metav1.Now(),
			Message:   "BMC Request submitted",
		},
	}
)

func Test_ConditionsExist(t *testing.T) {
	ok := ConditionExists(testConditionData, seederv1alpha1.BMCObjectCreated)
	assert.True(t, ok, "expected condition to be found")
}

func Test_ConditionsExist_False(t *testing.T) {
	ok := ConditionExists(testConditionData, seederv1alpha1.BMCJobSubmitted)
	assert.False(t, ok, "expected condition to be not found")
}

func Test_RemoveCondition(t *testing.T) {
	newConditions := RemoveCondition(testConditionData, seederv1alpha1.BMCObjectCreated)
	ok := ConditionExists(newConditions, seederv1alpha1.BMCObjectCreated)
	assert.False(t, ok, "expected condition to be not found")
}

func Test_AddCondition(t *testing.T) {
	newConditions := CreateOrUpdateCondition(testConditionData, seederv1alpha1.BMCJobComplete, "task completed")
	ok := ConditionExists(newConditions, seederv1alpha1.BMCJobComplete)
	assert.True(t, ok, "expected new condition to be present")
	ok = ConditionExists(newConditions, seederv1alpha1.BMCObjectCreated)
	assert.True(t, ok, "expected original condition to be present", newConditions)
}

func Test_UpdateCondition(t *testing.T) {
	orgTime := testConditionData[0].StartTime
	newConditions := CreateOrUpdateCondition(testConditionData, seederv1alpha1.BMCObjectCreated, "new task request")
	ok := ConditionExists(newConditions, seederv1alpha1.BMCObjectCreated)
	assert.True(t, ok, "expected condition to be present")
	assert.Equal(t, orgTime, newConditions[0].StartTime, "original time should be unchanged")
	assert.NotEmpty(t, newConditions[0].LastUpdateTime, "lastUpdateTime should not be empty")
}
