package xcontrollerutil

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Object interface {
	SetConditions(conditions []metav1.Condition)
	GetConditions() []metav1.Condition
}

func AddCondition(obj Object, condition metav1.Condition) bool {
	var (
		conditions    = obj.GetConditions()
		lenConditions = len(conditions)
	)
	if lenConditions == 0 {
		condition.LastTransitionTime.Time = time.Now()
		obj.SetConditions([]metav1.Condition{condition})
		return true
	} else if lastCondition := conditions[lenConditions-1]; lastCondition.Reason != condition.Reason || lastCondition.Message != condition.Message || lastCondition.Type != condition.Type || lastCondition.Status != condition.Status {
		condition.LastTransitionTime.Time = time.Now()
		conditions = append(conditions, condition)
		obj.SetConditions(conditions)
		return true
	}

	return false
}
