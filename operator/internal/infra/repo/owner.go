package repo

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// OwnerSetterImpl implements OwnerSetter using controller-runtime's SetControllerReference.
type OwnerSetterImpl struct {
	Scheme *runtime.Scheme
}

func (o *OwnerSetterImpl) SetOwner(owner, controlled metav1.Object) error {
	return controllerutil.SetControllerReference(owner, controlled, o.Scheme)
}
