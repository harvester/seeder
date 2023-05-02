package util

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
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
	assert := require.New(t)
	ok := ConditionExists(testConditionData, seederv1alpha1.BMCObjectCreated)
	assert.True(ok, "expected condition to be found")
}

func Test_ConditionsExist_False(t *testing.T) {
	assert := require.New(t)
	ok := ConditionExists(testConditionData, seederv1alpha1.BMCJobSubmitted)
	assert.False(ok, "expected condition to be not found")
}

func Test_RemoveCondition(t *testing.T) {
	assert := require.New(t)
	newConditions := RemoveCondition(testConditionData, seederv1alpha1.BMCObjectCreated)
	ok := ConditionExists(newConditions, seederv1alpha1.BMCObjectCreated)
	assert.False(ok, "expected condition to be not found")
}

func Test_AddCondition(t *testing.T) {
	assert := require.New(t)
	newConditions := CreateOrUpdateCondition(testConditionData, seederv1alpha1.BMCJobComplete, "task completed")
	ok := ConditionExists(newConditions, seederv1alpha1.BMCJobComplete)
	assert.True(ok, "expected new condition to be present")
	ok = ConditionExists(newConditions, seederv1alpha1.BMCObjectCreated)
	assert.True(ok, "expected original condition to be present", newConditions)
}

func Test_UpdateCondition(t *testing.T) {
	assert := require.New(t)
	orgTime := testConditionData[0].StartTime
	newConditions := CreateOrUpdateCondition(testConditionData, seederv1alpha1.BMCObjectCreated, "new task request")
	ok := ConditionExists(newConditions, seederv1alpha1.BMCObjectCreated)
	assert.True(ok, "expected condition to be present")
	assert.Equal(orgTime, newConditions[0].StartTime, "original time should be unchanged")
	assert.NotEmpty(newConditions[0].LastUpdateTime, "lastUpdateTime should not be empty")
}
