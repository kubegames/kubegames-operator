package game

import (
	"context"
	"fmt"
	"time"

	gameservice "github.com/kubegames/kubegames-operator/app/game"
	"github.com/kubegames/kubegames-operator/app/game/types"
	"github.com/kubegames/kubegames-operator/internal/pkg/log"
	gamesv1 "github.com/kubegames/kubegames-operator/pkg/apis/game/v1"
	gamesclientset "github.com/kubegames/kubegames-operator/pkg/client/game/clientset/versioned"
	"github.com/kubegames/kubegames-operator/pkg/client/game/clientset/versioned/scheme"
	gamesscheme "github.com/kubegames/kubegames-operator/pkg/client/game/clientset/versioned/scheme"
	factory "github.com/kubegames/kubegames-operator/pkg/client/game/informers/externalversions"
	informers "github.com/kubegames/kubegames-operator/pkg/client/game/informers/externalversions/game/v1"
	"github.com/kubegames/kubegames-operator/pkg/tools"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Game is the game implementation for Game resources
type Game struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// gamesclientset is a clientset for our own API group
	gamesclientset gamesclientset.Interface
	//informer
	informer informers.GameInformer
	//queue
	workqueue workqueue.DelayingInterface
	//factory
	factory factory.SharedInformerFactory
}

// returns a new game
func NewGame(kubeclientset kubernetes.Interface, config *rest.Config) *Game {

	//new game client set
	gamesclientset, err := gamesclientset.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	//new game factory
	factory := factory.NewSharedInformerFactory(gamesclientset, time.Second*15)

	runtime.Must(gamesscheme.AddToScheme(scheme.Scheme))

	game := &Game{
		kubeclientset:  kubeclientset,
		gamesclientset: gamesclientset,
		workqueue:      workqueue.NewDelayingQueue(),
		informer:       factory.Kubegames().V1().Games(),
		factory:        factory,
	}

	//listen game change event
	game.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				log.Errorf("add Func error %s", err.Error())
				return
			}
			game.workqueue.Add(key)
		},
		UpdateFunc: func(old, new interface{}) {
			oldgame := old.(*gamesv1.Game)
			newgame := new.(*gamesv1.Game)
			if oldgame.ResourceVersion != newgame.ResourceVersion {
				key, err := cache.MetaNamespaceKeyFunc(new)
				if err != nil {
					log.Errorf("update Func error %s", err.Error())
					return
				}
				game.workqueue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				log.Errorf("delete Func error %s", err.Error())
				return
			}
			game.workqueue.Add(key)
		},
	})

	return game
}

//run
func (c *Game) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	//start factory
	go c.factory.Start(stopCh)

	if ok := cache.WaitForCacheSync(stopCh, c.informer.Informer().HasSynced); !ok {
		panic("failed to wait for caches to sync")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Infoln("game controller start")
	<-stopCh
	log.Infoln("game controller end")
	return
}

func (c *Game) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem
func (c *Game) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	//done obj
	c.workqueue.Done(obj)

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
func (c *Game) syncHandler(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return nil
	}

	// get games
	game, err := c.informer.Lister().Games(namespace).Get(name)
	if err != nil {
		// delete
		if errors.IsNotFound(err) {
			//delete config map
			if err := c.kubeclientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
				if errors.IsNotFound(err) == false {
					log.Errorf("delete configmap error %s", err.Error())
				}
				return err
			}

			log.Tracef("game delete %s/%s", namespace, name)
			return nil
		}

		log.Errorf("failed to list games by: %s/%s", namespace, name)
		return err
	}

	// check DeletionTimestamp
	if game.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Tracef("add or update games %s/%s", namespace, name)

		if err := c.updateGames(ctx, game); err != nil {
			log.Errorf("update games %s/%s error %s", namespace, name, err.Error())
			return err
		}
	} else {
		log.Tracef("delete games %s/%s", namespace, name)

		if err := c.deleteGames(ctx, game); err != nil {
			log.Errorf("delete games %s/%s error %s", namespace, name, err.Error())
			return err
		}
	}

	log.Tracef("sync games %s succcess", game.Name)
	return nil
}

func (c *Game) deleteGames(ctx context.Context, game *gamesv1.Game) error {
	if tools.ContainsString(game.ObjectMeta.Finalizers, tools.Finalizer) {

		//get pod
		pods, err := c.kubeclientset.CoreV1().Pods(game.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(map[string]string{tools.LabelsGameID: game.Spec.GameID}),
		})
		if err != nil {
			log.Errorf("get pod list error %s", err.Error())
			return err
		}

		//check pod items
		if len(pods.Items) <= 0 {
			//update games finalizer
			game.ObjectMeta.Finalizers = tools.RemoveString(game.ObjectMeta.Finalizers, tools.Finalizer)
			if _, err := c.gamesclientset.KubegamesV1().Games(game.Namespace).Update(ctx, game, metav1.UpdateOptions{}); err != nil {
				log.Errorf("update games error %s", err.Error())
				return err
			}
			return nil
		}

		for _, pod := range pods.Items {
			log.Tracef("reduce - game pod %s", pod.Name)

			if err := c.reduceGamePod(ctx, &pod, game); err != nil {
				log.Errorf("reduce - game pod %s error %s", pod.Name, err.Error())
			}
		}
		return fmt.Errorf("wait pod all close")
	}
	return nil
}

func (c *Game) updateGames(ctx context.Context, game *gamesv1.Game) error {
	//get create pod number
	number := uint32(len(game.Status.Pods))

	if number == game.Spec.Replicas {
		log.Tracef("game %s/%s spec == status", game.Namespace, game.Name)
		return nil
	}

	//chek game namespace
	if _, err := c.kubeclientset.CoreV1().Namespaces().Get(ctx, game.Namespace, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) == false {
			log.Errorf("get namespace error %s", err.Error())
			return err
		}

		//create namespace
		ns := &corev1.Namespace{}
		ns.Name = game.Namespace
		if _, err := c.kubeclientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
			log.Errorf("create namespace error %s", err.Error())
			return err
		}
	}

	//check games config
	if _, err := c.kubeclientset.CoreV1().ConfigMaps(game.Namespace).Get(ctx, game.Spec.GameID, metav1.GetOptions{}); err != nil {
		if errors.IsNotFound(err) == false {
			log.Errorf("get configmap error %s", err.Error())
			return err
		}

		//create config map
		if _, err := c.kubeclientset.CoreV1().ConfigMaps(game.Namespace).Create(ctx, tools.CreateConfigMap(game), metav1.CreateOptions{}); err != nil {
			log.Errorf("create configmap error %s", err.Error())
			return err
		}
	}

	//pod +
	if number < game.Spec.Replicas {
		//+ pod
		podname := fmt.Sprintf("%s-%d", game.Spec.GameID, number)

		log.Tracef("increase + game pod %s", podname)

		//increase game pod
		if err := c.increaseGamePod(ctx, number, podname, game); err != nil {
			log.Errorf("increase + game pod error %s", err.Error())
			return err
		}
	}

	//pod -
	if number > game.Spec.Replicas {
		//- pod
		podname := fmt.Sprintf("%s-%d", game.Spec.GameID, number-1)

		//get pods
		pod, err := c.kubeclientset.CoreV1().Pods(game.Namespace).Get(ctx, podname, metav1.GetOptions{})
		if err != nil {
			log.Errorf("get pod %s/%s error %s", game.Namespace, podname, err.Error())
			return err
		}

		log.Tracef("reduce - game pod %s", podname)

		//reduce game pod
		if err := c.reduceGamePod(ctx, pod, game); err != nil {
			log.Errorf("reduce - game pod error %s", err.Error())
			return err
		}
	}
	return nil
}

//increase game pod +
func (c *Game) increaseGamePod(ctx context.Context, number uint32, podname string, game *gamesv1.Game) error {
	//create pod
	if _, err := c.kubeclientset.CoreV1().Pods(game.Namespace).Create(ctx, tools.CreatePod(podname, game), metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) == false {
			log.Errorf("create pod error %s", err.Error())
			return err
		}
	}
	return nil
}

//reduce game pod -
func (c *Game) reduceGamePod(ctx context.Context, pod *corev1.Pod, game *gamesv1.Game) error {
	//check pod is running
	if pod.Status.Phase == corev1.PodRunning && pod.ObjectMeta.DeletionTimestamp.IsZero() {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.ContainersReady && condition.Status == corev1.ConditionTrue {
				//call server delete
				ok, err := c.deleteCall(ctx, fmt.Sprintf("%s:%d", pod.Status.PodIP, game.Spec.Port), game.Spec.GameID)
				if err == nil && ok == false {
					log.Tracef("wait delete pod %s", pod.Name)
					return fmt.Errorf("wait delete pod %s", pod.Name)
				}
				break
			}
		}
	}

	if err := c.kubeclientset.EventsV1().Events(game.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		FieldSelector: fields.Set{"regarding.name": pod.Name}.String(),
	}); err != nil {
		if errors.IsNotFound(err) == false {
			log.Errorf("get event %s", err.Error())
			return err
		}
	}

	//delete pod
	if err := c.kubeclientset.CoreV1().Pods(game.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) == false {
			log.Errorf("delete pod error %s", err.Error())
			return err
		}
	}

	return nil
}

//defalt call rpc
func (c *Game) deleteCall(ctx context.Context, address string, gameID string) (bool, error) {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Errorf("did not connect: %v", err)
		return false, err
	}
	defer conn.Close()
	client := gameservice.NewGameServiceClient(conn)
	resp, err := client.Delete(ctx, &types.DeleteRequest{GameID: gameID})
	if err != nil {
		log.Errorf("grpc delete call error %s", err.Error())
		return false, err
	}
	return resp.Success, nil
}
