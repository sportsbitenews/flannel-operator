package namespace

import (
	"context"
	"testing"

	"github.com/giantswarm/flanneltpr"
	flanneltprspec "github.com/giantswarm/flanneltpr/spec"
	"github.com/giantswarm/micrologger/microloggertest"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	apiv1 "k8s.io/client-go/pkg/api/v1"
)

func Test_Resource_Namespace_GetDeleteState(t *testing.T) {
	testCases := []struct {
		Obj               interface{}
		Cur               interface{}
		Des               interface{}
		ExpectedNamespace *apiv1.Namespace
	}{
		{
			Obj: &flanneltpr.CustomObject{
				Spec: flanneltpr.Spec{
					Cluster: flanneltprspec.Cluster{
						ID: "foobar",
					},
				},
			},
			Cur: &apiv1.Namespace{
				TypeMeta: apismetav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: apismetav1.ObjectMeta{
					Name: "al9qy",
					Labels: map[string]string{
						"cluster":  "al9qy",
						"customer": "test-customer",
					},
				},
			},
			Des: &apiv1.Namespace{
				TypeMeta: apismetav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: apismetav1.ObjectMeta{
					Name: "al9qy",
					Labels: map[string]string{
						"cluster":  "al9qy",
						"customer": "test-customer",
					},
				},
			},
			ExpectedNamespace: &apiv1.Namespace{
				TypeMeta: apismetav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: apismetav1.ObjectMeta{
					Name: "al9qy",
					Labels: map[string]string{
						"cluster":  "al9qy",
						"customer": "test-customer",
					},
				},
			},
		},

		{
			Obj: &flanneltpr.CustomObject{
				Spec: flanneltpr.Spec{
					Cluster: flanneltprspec.Cluster{
						ID: "foobar",
					},
				},
			},
			Cur: nil,
			Des: &apiv1.Namespace{
				TypeMeta: apismetav1.TypeMeta{
					Kind:       "Namespace",
					APIVersion: "v1",
				},
				ObjectMeta: apismetav1.ObjectMeta{
					Name: "al9qy",
					Labels: map[string]string{
						"cluster":  "al9qy",
						"customer": "test-customer",
					},
				},
			},
			ExpectedNamespace: nil,
		},
	}

	var err error
	var newResource *Resource
	{
		resourceConfig := DefaultConfig()
		resourceConfig.K8sClient = fake.NewSimpleClientset()
		resourceConfig.Logger = microloggertest.New()
		newResource, err = New(resourceConfig)
		if err != nil {
			t.Fatal("expected", nil, "got", err)
		}
	}

	for i, tc := range testCases {
		result, err := newResource.GetDeleteState(context.TODO(), tc.Obj, tc.Cur, tc.Des)
		if err != nil {
			t.Fatal("case", i+1, "expected", nil, "got", err)
		}
		if tc.ExpectedNamespace == nil {
			if tc.ExpectedNamespace != result {
				t.Fatal("case", i+1, "expected", tc.ExpectedNamespace, "got", result)
			}
		} else {
			name := result.(*apiv1.Namespace).Name
			if tc.ExpectedNamespace.Name != name {
				t.Fatal("case", i+1, "expected", tc.ExpectedNamespace.Name, "got", name)
			}
		}
	}
}
