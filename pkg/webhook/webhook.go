package webhook

import (
	"encoding/json"
	"fmt"

	"github.com/kubegames/kubegames-operator/internal/pkg/log"
	gamev1 "github.com/kubegames/kubegames-operator/pkg/apis/game/v1"
	"github.com/kubegames/kubegames-operator/pkg/convert"
	"github.com/kubegames/kubegames-operator/pkg/scheme"
	"github.com/kubegames/kubegames-operator/pkg/tools"
	"github.com/wI2L/jsondiff"
	v1 "k8s.io/api/admission/v1"
)

// validate
func Validating(ar v1.AdmissionReview) *v1.AdmissionResponse {
	req := ar.Request
	if req.Operation != "CREATE" {
		return &v1.AdmissionResponse{Allowed: true}
	}

	log.Tracef("Validate %s Kind=%v, Namespace=%v Name=%v", req.Operation, req.Kind, req.Namespace, req.Name)

	switch req.Kind.Kind {
	case "Game":
		game := new(gamev1.Game)
		deserializer := scheme.Codecs.UniversalDeserializer()
		if _, _, err := deserializer.Decode(req.Object.Raw, nil, game); err != nil {
			log.Errorln(err)
			return convert.ToV1AdmissionResponse(err)
		}
		return ValidatingGame(game)
	}

	return &v1.AdmissionResponse{Allowed: true}
}

func ValidatingGame(game *gamev1.Game) *v1.AdmissionResponse {
	if len(game.Spec.GameID) <= 0 {
		err := fmt.Errorf("game.spec.gameID is null !")
		log.Errorln()
		return convert.ToV1AdmissionResponse(err)
	}

	if len(game.Spec.Image) <= 0 {
		err := fmt.Errorf("game.spec.image is null !")
		log.Errorln(err.Error())
		return convert.ToV1AdmissionResponse(err)
	}

	if tools.ContainsString(game.ObjectMeta.Finalizers, tools.Finalizer) == false {
		err := fmt.Errorf("game.objectmeta.finalizers is null !")
		log.Errorln(err.Error())
		return convert.ToV1AdmissionResponse(err)
	}

	reviewResponse := v1.AdmissionResponse{}
	reviewResponse.Allowed = true
	return &reviewResponse
}

//mutating
func Mutating(ar v1.AdmissionReview) *v1.AdmissionResponse {
	req := ar.Request
	if req.Operation != "CREATE" {
		return &v1.AdmissionResponse{Allowed: true}
	}

	log.Tracef("Mutating %s Kind=%v, Namespace=%v Name=%v", req.Operation, req.Kind, req.Namespace, req.Name)

	switch req.Kind.Kind {
	case "Game":
		game := new(gamev1.Game)
		deserializer := scheme.Codecs.UniversalDeserializer()
		if _, _, err := deserializer.Decode(req.Object.Raw, nil, game); err != nil {
			log.Errorln(err)
			return convert.ToV1AdmissionResponse(err)
		}
		return MutatingGame(game)
	}

	return &v1.AdmissionResponse{Allowed: true}
}

func MutatingGame(game *gamev1.Game) *v1.AdmissionResponse {
	//new game
	newgame := game.DeepCopy()

	if len(newgame.Labels) <= 0 {
		newgame.Labels = make(map[string]string)
	}

	//set lables
	newgame.Labels[tools.LabelsGameID] = newgame.Spec.GameID
	newgame.Labels[tools.LabelsController] = tools.LabelsControllerValue

	//add finalizers
	newgame.ObjectMeta.Finalizers = append(newgame.ObjectMeta.Finalizers, tools.Finalizer)

	patch, err := jsondiff.Compare(game, newgame)
	if err != nil {
		log.Errorf("patch Compare process error: %v", err.Error())
		return convert.ToV1AdmissionResponse(err)
	}

	patchBytes, err := json.MarshalIndent(patch, "", "    ")
	if err != nil {
		log.Errorf("patch process error: %v", err.Error())
		return convert.ToV1AdmissionResponse(err)
	}

	log.Infof("game %s/%s patch=%v", game.Namespace, game.Name, string(patchBytes))

	return &v1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1.PatchType {
			pt := v1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}
