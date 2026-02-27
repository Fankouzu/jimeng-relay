package api

import "fmt"

const (
	ActionSubmitTask = "CVSync2AsyncSubmitTask"
	ActionGetResult  = "CVSync2AsyncGetResult"

	ReqKeyJimengT2IV40 = "jimeng_t2i_v40"

	ReqKeyJimengT2VV30_720p          = "jimeng_t2v_v30"
	ReqKeyJimengT2VV30_1080p         = "jimeng_t2v_v30_1080p"
	ReqKeyJimengI2VFirstV30           = "jimeng_i2v_first_v30"
	ReqKeyJimengI2VFirstV30_1080     = "jimeng_i2v_first_v30_1080"
	ReqKeyJimengI2VFirstTailV30       = "jimeng_i2v_first_tail_v30"
	ReqKeyJimengI2VFirstTailV30_1080 = "jimeng_i2v_first_tail_v30_1080"
	ReqKeyJimengI2VRecameraV30       = "jimeng_i2v_recamera_v30"

	ReqKeyJimengT2VV30Pro      = "jimeng_ti2v_v30_pro"
	ReqKeyJimengI2VFirstV30Pro = "jimeng_ti2v_v30_pro"
)
const (
	StatusInQueue    = "in_queue"
	StatusGenerating = "generating"
	StatusDone       = "done"
	StatusNotFound   = "not_found"
	StatusExpired    = "expired"
	StatusFailed     = "failed"
)

type FieldSpec struct {
	Name     string
	Type     string
	Required bool
	Note     string
}

type ApiActionSpec struct {
	CommandName     string
	Action          string
	ReqKey          string
	RequiredParams  []FieldSpec
	OptionalParams  []FieldSpec
	ResponseFields  []FieldSpec
	AllowedStatuses []string
}

type ApiMatrix struct {
	ActionOrder []string
	Actions     map[string]ApiActionSpec
}

func FrozenApiMatrix() ApiMatrix {
	statuses := []string{
		StatusInQueue,
		StatusGenerating,
		StatusDone,
		StatusNotFound,
		StatusExpired,
		StatusFailed,
	}

	return ApiMatrix{
		ActionOrder: []string{ActionSubmitTask, ActionGetResult},
		Actions: map[string]ApiActionSpec{
			ActionSubmitTask: {
				Action:          ActionSubmitTask,
				ReqKey:          ReqKeyJimengT2IV40,
				AllowedStatuses: statuses,
			},
			ActionGetResult: {
				Action:          ActionGetResult,
				ReqKey:          ReqKeyJimengT2IV40,
				AllowedStatuses: statuses,
			},
		},
	}
}

func (m ApiMatrix) ValidateAction(action string) error {
	if action == "" {
		return fmt.Errorf("action must not be empty")
	}

	for _, a := range m.ActionOrder {
		if a == action {
			return nil
		}
	}

	return fmt.Errorf("unsupported action: %s", action)
}

type VideoPreset string

const (
	VideoPresetT2V720          VideoPreset = "t2v-720"
	VideoPresetT2V1080         VideoPreset = "t2v-1080"
	VideoPresetI2VFirst720     VideoPreset = "i2v-first-720"
	VideoPresetI2VFirst        VideoPreset = "i2v-first"
	VideoPresetI2VFirstTail720 VideoPreset = "i2v-first-tail-720"
	VideoPresetI2VFirstTail    VideoPreset = "i2v-first-tail"
	VideoPresetI2VRecamera     VideoPreset = "i2v-recamera"
	VideoPresetT2VPro          VideoPreset = "t2v-pro"
	VideoPresetI2VFirstPro     VideoPreset = "i2v-first-pro"
)

type PresetCapabilities struct {
	Supported          bool
	RequiresImage      bool
	AcceptsImage       bool
	AcceptsFrames      bool
	AcceptsTemplate    bool
	MinImageCount      int
	MaxImageCount      int
	DefaultFrames      int
	DefaultAspectRatio string
}

var presetCapabilities = map[VideoPreset]PresetCapabilities{
	VideoPresetT2V720: {
		Supported:          true,
		AcceptsFrames:      true,
		DefaultFrames:      121,
		DefaultAspectRatio: "16:9",
	},
	VideoPresetT2V1080: {
		Supported:          true,
		AcceptsFrames:      true,
		DefaultFrames:      121,
		DefaultAspectRatio: "16:9",
	},
	VideoPresetT2VPro: {
		Supported:          true,
		AcceptsFrames:      true,
		DefaultFrames:      121,
		DefaultAspectRatio: "16:9",
	},
VideoPresetI2VFirst: {
Supported:     true,
RequiresImage: true,
AcceptsImage:  true,
MinImageCount: 1,
MaxImageCount: 1,
},
VideoPresetI2VFirst720: {
Supported:     true,
RequiresImage: true,
AcceptsImage:  true,
MinImageCount: 1,
MaxImageCount: 1,
	},
VideoPresetI2VFirstPro: {
Supported:     true,
RequiresImage: true,
AcceptsImage:  true,
MinImageCount: 1,
MaxImageCount: 1,
	},
	VideoPresetI2VFirstTail720: {
		Supported:     true,
		RequiresImage: true,
		AcceptsImage:  true,
		MinImageCount: 2,
		MaxImageCount: 2,
	},
VideoPresetI2VFirstTail: {
Supported:     true,
RequiresImage: true,
AcceptsImage:  true,
MinImageCount: 2,
MaxImageCount: 2,
},
VideoPresetI2VRecamera: {
Supported:       true,
RequiresImage:   true,
AcceptsImage:    true,
AcceptsTemplate: true,
MinImageCount:   1,
MaxImageCount:   1,
},
}

func GetPresetCapabilities(preset VideoPreset) PresetCapabilities {
	if cap, ok := presetCapabilities[preset]; ok {
		return cap
	}

	return PresetCapabilities{}
}

func VideoReqKeyForPreset(preset VideoPreset) (string, error) {
switch preset {
case VideoPresetT2V720:
return ReqKeyJimengT2VV30_720p, nil
case VideoPresetT2V1080:
		return ReqKeyJimengT2VV30_1080p, nil
	case VideoPresetI2VFirst720:
		return ReqKeyJimengI2VFirstV30, nil
case VideoPresetI2VFirst:
		return ReqKeyJimengI2VFirstV30_1080, nil
	case VideoPresetI2VFirstTail720:
		return ReqKeyJimengI2VFirstTailV30, nil
case VideoPresetI2VFirstTail:
return ReqKeyJimengI2VFirstTailV30_1080, nil
case VideoPresetI2VRecamera:
return ReqKeyJimengI2VRecameraV30, nil
case VideoPresetT2VPro:
return ReqKeyJimengT2VV30Pro, nil
case VideoPresetI2VFirstPro:
return ReqKeyJimengI2VFirstV30Pro, nil
default:
return "", fmt.Errorf("unsupported video preset: %q", preset)
}
}

func VideoSubmitReqKey(preset VideoPreset) (string, error) {
	return VideoReqKeyForPreset(preset)
}

func VideoQueryReqKey(preset VideoPreset) (string, error) {
	return VideoReqKeyForPreset(preset)
}

// ValidatePresetCombination checks if the preset and input combination is valid.
func ValidatePresetCombination(preset VideoPreset, hasImage bool) error {
	capabilities := GetPresetCapabilities(preset)
	if !capabilities.Supported {
		return fmt.Errorf("unsupported video preset: %q", preset)
	}

	if hasImage && !capabilities.AcceptsImage {
		return fmt.Errorf("preset %q does not support image input (text-to-video)", preset)
	}
	if !hasImage && capabilities.RequiresImage {
		return fmt.Errorf("preset %q requires image input (image-to-video)", preset)
	}

	return nil
}
