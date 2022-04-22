package pod

import (
	"context"
	"strings"
	"time"

	"github.com/kubegames/kubegames-operator/internal/pkg/log"
	gamesv1 "github.com/kubegames/kubegames-operator/pkg/apis/game/v1"
	gamesclientset "github.com/kubegames/kubegames-operator/pkg/client/game/clientset/versioned"
	"github.com/kubegames/kubegames-operator/pkg/tools"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	podsv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type (
	//pod object
	Pod struct {
		// kubeclientset is a standard kubernetes clientset
		kubeclientset kubernetes.Interface
		//informer
		informer podsv1.PodInformer
		//factory
		factory informers.SharedInformerFactory
		//queue
		workqueue workqueue.DelayingInterface
		// gamesclientset is a clientset for our own API group
		gamesclientset gamesclientset.Interface
	}
)

//new pod
func NewPod(kubeclientset kubernetes.Interface, config *rest.Config) *Pod {
	//new game client set
	gamesclientset, err := gamesclientset.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	//new factory
	factory := informers.NewSharedInformerFactory(kubeclientset, time.Second*15)

	//new pod
	pod := &Pod{
		kubeclientset:  kubeclientset,
		informer:       factory.Core().V1().Pods(),
		factory:        factory,
		workqueue:      workqueue.NewDelayingQueue(),
		gamesclientset: gamesclientset,
	}

	//listen game change event
	pod.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			objpod := obj.(*corev1.Pod)
			value, ok := objpod.Labels[tools.LabelsController]
			if !ok {
				return
			}
			if value != tools.LabelsControllerValue {
				return
			}

			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				log.Errorf("add error %s", err.Error())
				return
			}
			pod.workqueue.Add(key)
		},
		UpdateFunc: func(old, new interface{}) {
			oldpod := old.(*corev1.Pod)
			newpod := new.(*corev1.Pod)
			value, ok := newpod.Labels[tools.LabelsController]
			if !ok {
				return
			}
			if value != tools.LabelsControllerValue {
				return
			}

			if oldpod.ResourceVersion != newpod.ResourceVersion {
				key, err := cache.MetaNamespaceKeyFunc(new)
				if err != nil {
					log.Errorf("update error %s", err.Error())
					return
				}
				pod.workqueue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			objpod := obj.(*corev1.Pod)
			value, ok := objpod.Labels[tools.LabelsController]
			if !ok {
				return
			}
			if value != tools.LabelsControllerValue {
				return
			}
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				log.Errorf("delete Func error %s", err.Error())
				return
			}
			pod.workqueue.Add(key)
		},
	})
	return pod
}

//run
func (c *Pod) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	go c.factory.Start(stopCh)

	if ok := cache.WaitForCacheSync(stopCh, c.informer.Informer().HasSynced); !ok {
		panic("failed to wait for caches to sync")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Infoln("pod controller start")
	<-stopCh
	log.Infoln("pod controller end")
	return
}

func (c *Pod) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem
func (c *Pod) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	//done obj
	c.workqueue.Done(obj)

	//to key
	key, ok := obj.(string)
	if !ok {
		log.Errorf("expected string in workqueue but got %#v", obj)
		return true
	}

	// handler
	if err := c.syncHandler(context.Background(), key); err != nil {
		c.workqueue.AddAfter(obj, time.Second*15)
		return false
	}

	return true
}

// handler
func (c *Pod) syncHandler(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return nil
	}

	// get pod
	pod, err := c.informer.Lister().Pods(namespace).Get(name)
	if err != nil {
		// delete
		if errors.IsNotFound(err) == false {
			log.Errorf("failed to list rooms by: %s/%s", namespace, name)
			return err
		}

		return c.deletePods(ctx, namespace, name)
	}

	if pod.Status.Phase != corev1.PodRunning || pod.ObjectMeta.DeletionTimestamp.IsZero() == false {
		return nil
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.ContainersReady && condition.Status != corev1.ConditionTrue {
			return nil
		}
	}

	log.Tracef("pod add or update rooms %s/%s phase %s", namespace, name, pod.Status.Phase)

	if err := c.updatePods(ctx, pod); err != nil {
		log.Errorf("update pod %s/%s error %s", namespace, name, err.Error())
		return err
	}

	log.Tracef("sync pod %s succcess", pod.Name)
	return nil
}

func (c *Pod) updatePods(ctx context.Context, pod *corev1.Pod) error {
	log.Tracef("notice pod %s open rooms", pod.Name)
	if gameid, ok := pod.Labels[tools.LabelsGameID]; ok {
		//game not found
		game, err := c.gamesclientset.KubegamesV1().Games(pod.Namespace).Get(ctx, gameid, v1.GetOptions{})
		if err != nil {
			log.Errorf("get game %s/%s error %s", pod.Namespace, gameid, err.Error())
			return err
		}

		//init game status pods
		if len(game.Status.Pods) <= 0 {
			game.Status.Pods = make(map[string]*gamesv1.PodStatus)
		}

		//create pod
		podstatus := &gamesv1.PodStatus{
			Name:   pod.Name,
			HostIP: pod.Status.HostIP,
			PodIP:  pod.Status.PodIP,
			Port:   game.Spec.Port,
			Phase:  pod.Status.Phase,
			Events: make([]string, 0),
		}

		//get events
		events, err := c.kubeclientset.EventsV1().Events(pod.Namespace).List(ctx, v1.ListOptions{
			FieldSelector: fields.Set{"regarding.name": pod.Name}.String(),
		})
		if err != nil {
			log.Errorf("get event %s", err.Error())
			return err

		}

		//add events
		for _, event := range events.Items {
			podstatus.Events = append(podstatus.Events, event.Note)
		}

		//set pod status
		game.Status.Pods[pod.Name] = podstatus

		//set update
		game.Status.UpdateAt = time.Now().String()

		//update
		if _, err := c.gamesclientset.KubegamesV1().Games(game.Namespace).Update(ctx, game, v1.UpdateOptions{}); err != nil {
			log.Errorf("update games %s/%s status error %s", game.Namespace, game.Name, err.Error())
			return err
		}

		log.Tracef("update games %s/%s status", game.Name, game.Namespace)
	}

	return nil
}

func (c *Pod) deletePods(ctx context.Context, namespace, name string) error {
	log.Tracef("pod delete %s/%s", namespace, name)

	//delete pod
	array := strings.Split(name, "-")
	if len(array) > 1 {
		//game not found
		game, err := c.gamesclientset.KubegamesV1().Games(namespace).Get(ctx, array[0], v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) == true {
				return nil
			}
			log.Errorf("get game %s/%s error %s", namespace, array[0], err.Error())
			return err
		}

		//check game is delete
		if game.ObjectMeta.DeletionTimestamp.IsZero() {
			//delete
			delete(game.Status.Pods, name)

			//set update
			game.Status.UpdateAt = time.Now().String()

			//update
			if _, err := c.gamesclientset.KubegamesV1().Games(game.Namespace).UpdateStatus(ctx, game, v1.UpdateOptions{}); err != nil {
				log.Errorf("update games %s/%s status error %s", game.Name, game.Namespace, err.Error())
				return err
			}

			log.Tracef("update games %s/%s status", game.Name, game.Namespace)
		}
	}
	return nil
}
