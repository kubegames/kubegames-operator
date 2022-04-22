package tools

import (
	"crypto/md5"
	"fmt"
	"io"

	"github.com/kubegames/kubegames-operator/internal/pkg/log"
	gamesv1 "github.com/kubegames/kubegames-operator/pkg/apis/game/v1"
	"github.com/kubegames/kubegames-proxy/pkg/route"
	coreV1 "k8s.io/api/core/v1"
	rs "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	MountPath             = "/game/config"
	MountConfigName       = "config"
	LabelsGameID          = "gameid"
	LabelsProxy           = "proxy"
	RunPort               = "RUN_PORT"
	PodIp                 = "POD_IP"
	PodName               = "POD_NAME"
	LabelsController      = "controller"
	LabelsControllerValue = "kubegames"
	LabelsPort            = "port"
)

//create configmap
func CreateConfigMap(game *gamesv1.Game) *coreV1.ConfigMap {
	cm := &coreV1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      game.Spec.GameID,
			Namespace: game.Namespace,
			Labels: map[string]string{
				LabelsGameID:     game.Spec.GameID,
				LabelsController: LabelsControllerValue,
			},
		},
		Data: map[string]string{
			MountConfigName: game.Spec.Config,
		},
	}
	return cm
}

func CreatePod(podname string, game *gamesv1.Game) *coreV1.Pod {
	volume := coreV1.Volume{Name: game.Spec.GameID}
	volume.ConfigMap = &coreV1.ConfigMapVolumeSource{}
	volume.ConfigMap.Name = game.Spec.GameID

	//volumes
	volumes := []coreV1.Volume{volume}

	//volume mount
	volumeMount := coreV1.VolumeMount{
		MountPath: MountPath,
		Name:      game.Spec.GameID,
	}

	//volume mounts
	volumeMounts := []coreV1.VolumeMount{volumeMount}

	//limit
	limit := coreV1.ResourceList{}

	//cpu
	if game.Spec.Cpu > 0 {
		c := rs.NewMilliQuantity(int64(game.Spec.Cpu), rs.DecimalSI)
		limit[coreV1.ResourceCPU] = *c
	}

	//memory
	if game.Spec.Memory > 0 {
		m := rs.NewQuantity(int64(game.Spec.Memory*1024*1024), rs.BinarySI)
		limit[coreV1.ResourceMemory] = *m
	}

	//create rules
	rules := route.NewRules(route.Pod, route.NewRule(route.Any, fmt.Sprintf("/%s/%s", game.Spec.GameID, podname), "", int64(game.Spec.Port)))
	base64, err := route.Marshal(rules)
	if err != nil {
		log.Errorf("create rules error %s", err.Error())
	}

	//readinessProbe
	readinessProbe := &coreV1.Probe{
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
	}
	readinessProbe.TCPSocket = &coreV1.TCPSocketAction{
		Port: intstr.FromInt(int(game.Spec.Port)),
	}

	livenessProbe := &coreV1.Probe{
		InitialDelaySeconds: 20,
		PeriodSeconds:       10,
	}
	livenessProbe.TCPSocket = &coreV1.TCPSocketAction{
		Port: intstr.FromInt(int(game.Spec.Port)),
	}

	//create pod
	pod := coreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podname,
			Namespace: game.Namespace,
			Labels: map[string]string{
				LabelsGameID:     game.Spec.GameID,
				LabelsController: LabelsControllerValue,
				LabelsPort:       fmt.Sprintf("%d", game.Spec.Port),
			},
			Annotations: map[string]string{
				LabelsProxy: base64,
			},
		},
		Spec: coreV1.PodSpec{
			Containers: []coreV1.Container{
				{
					Name:            game.Spec.GameID,
					Image:           game.Spec.Image,
					ImagePullPolicy: coreV1.PullIfNotPresent,
					Command:         game.Spec.Commonds,
					Ports: []coreV1.ContainerPort{
						{
							Protocol:      coreV1.ProtocolTCP,
							ContainerPort: int32(game.Spec.Port),
						},
					},
					Resources: coreV1.ResourceRequirements{
						Limits: limit,
					},
					VolumeMounts: volumeMounts,
					Env: []coreV1.EnvVar{
						{
							Name:  RunPort,
							Value: fmt.Sprintf("%d", game.Spec.Port),
						},
						{
							Name:  PodName,
							Value: podname,
						},
						{
							Name:      PodIp,
							ValueFrom: &coreV1.EnvVarSource{FieldRef: &coreV1.ObjectFieldSelector{FieldPath: "status.podIP"}},
						},
					},
					ReadinessProbe: readinessProbe,
					LivenessProbe:  livenessProbe,
				},
			},
			Volumes: volumes,
		},
	}
	return &pod
}

func Md5(str string) string {
	w := md5.New()
	io.WriteString(w, str)
	md5str := fmt.Sprintf("%x", w.Sum(nil))
	return md5str
}
