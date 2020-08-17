package kbkb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKbkb(t *testing.T) {
	t.Run("success erasableKbkbPodList", func(t *testing.T) {
		kf := getKbkbField()
		assert.Equal(t, 7, len(kf.ErasableKbkbPodList(3)))
		assert.Equal(t, 4, len(kf.ErasableKbkbPodList(4)))
	})
}

func getKbkbField() KbkbField {
	node1 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.August, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	node2 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node2",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.August, 2, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	node3 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node3",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.August, 3, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	pod1 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod1",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 1, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "red",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node1",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod2 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod2",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 2, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "red",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node2",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod3 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod3",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 3, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "red",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node3",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod4 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod4",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 4, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "blue",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node1",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod5 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod5",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 5, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "blue",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node2",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod6 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod6",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 6, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "green",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node3",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod7 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod7",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 7, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "green",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node1",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod8 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod8",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 8, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "blue",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node2",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	pod9 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod9",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2020, time.September, 9, 0, 0, 0, 0, time.UTC),
			},
			Annotations: map[string]string{
				"kbkbColor": "blue",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "node3",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Ready: true,
				},
			},
		},
	}
	kf := BuildKbkbField([]*v1.Pod{&pod1, &pod2, &pod3, &pod4, &pod5, &pod6, &pod7, &pod8, &pod9}, []*v1.Node{&node1, &node2, &node3})
	printer := BashOverwritePrinter{Row: 0}
	kf.PrintAsKbkbOverwrite(&printer)
	return kf
}
