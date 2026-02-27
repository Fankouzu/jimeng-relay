package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApiMatrix(t *testing.T) {
	matrix := FrozenApiMatrix()

	if len(matrix.ActionOrder) != 2 {
		t.Fatalf("expected 2 actions in frozen matrix, got %d", len(matrix.ActionOrder))
	}

	for _, action := range []string{ActionSubmitTask, ActionGetResult} {
		spec, ok := matrix.Actions[action]
		if !ok {
			t.Fatalf("expected action %q in matrix", action)
		}

		if spec.ReqKey != ReqKeyJimengT2IV40 {
			t.Fatalf("expected req_key %q for %s, got %q", ReqKeyJimengT2IV40, action, spec.ReqKey)
		}

		expectedStatuses := map[string]bool{
			StatusInQueue:    true,
			StatusGenerating: true,
			StatusDone:       true,
			StatusNotFound:   true,
			StatusExpired:    true,
			StatusFailed:     true,
		}

		if len(spec.AllowedStatuses) != len(expectedStatuses) {
			t.Fatalf("expected %d statuses for %s, got %d", len(expectedStatuses), action, len(spec.AllowedStatuses))
		}

		for _, status := range spec.AllowedStatuses {
			if !expectedStatuses[status] {
				t.Fatalf("unexpected status %q for %s", status, action)
			}
		}
	}
}

func TestApiMatrixRejectsInvalidAction(t *testing.T) {
	matrix := FrozenApiMatrix()

	err := matrix.ValidateAction("CVProcess")
	if err == nil {
		t.Fatalf("expected invalid action to be rejected")
	}
}

func TestVideoPreset_ReqKeyConstantsMatchDocs(t *testing.T) {
	t.Parallel()

	require.Equal(t, "jimeng_t2v_v30", ReqKeyJimengT2VV30_720p)
	require.Equal(t, "jimeng_t2v_v30_1080p", ReqKeyJimengT2VV30_1080p)
	require.Equal(t, "jimeng_i2v_first_v30_1080", ReqKeyJimengI2VFirstV30_1080)
	require.Equal(t, "jimeng_i2v_first_tail_v30_1080", ReqKeyJimengI2VFirstTailV30_1080)
	require.Equal(t, "jimeng_i2v_recamera_v30", ReqKeyJimengI2VRecameraV30)
	require.Equal(t, "jimeng_ti2v_v30_pro", ReqKeyJimengT2VV30Pro)
	require.Equal(t, "jimeng_ti2v_v30_pro", ReqKeyJimengI2VFirstV30Pro)
}

func TestVideoPreset_ValidPresetsMapToReqKey(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		preset VideoPreset
		reqKey string
	}{
		{preset: VideoPresetT2V720, reqKey: ReqKeyJimengT2VV30_720p},
		{preset: VideoPresetT2V1080, reqKey: ReqKeyJimengT2VV30_1080p},
		{preset: VideoPresetI2VFirst, reqKey: ReqKeyJimengI2VFirstV30_1080},
		{preset: VideoPresetI2VFirstTail, reqKey: ReqKeyJimengI2VFirstTailV30_1080},
		{preset: VideoPresetI2VRecamera, reqKey: ReqKeyJimengI2VRecameraV30},
		{preset: VideoPresetT2VPro, reqKey: ReqKeyJimengT2VV30Pro},
		{preset: VideoPresetI2VFirstPro, reqKey: ReqKeyJimengI2VFirstV30Pro},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(string(tc.preset), func(t *testing.T) {
			t.Parallel()

			got, err := VideoReqKeyForPreset(tc.preset)
			require.NoError(t, err)
			require.Equal(t, tc.reqKey, got)
		})
	}
}

func TestVideoPreset_RejectsUnknownPreset(t *testing.T) {
	t.Parallel()

	_, err := VideoReqKeyForPreset(VideoPreset("nope"))
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported video preset")
}

func TestVideoPreset_QueryAndSubmitReqKeyConsistent(t *testing.T) {
	t.Parallel()

	for _, preset := range []VideoPreset{
		VideoPresetT2V720,
		VideoPresetT2V1080,
		VideoPresetI2VFirst,
		VideoPresetI2VFirstTail,
		VideoPresetI2VRecamera,
		VideoPresetT2VPro,
		VideoPresetI2VFirstPro,
	} {
		preset := preset
		t.Run(string(preset), func(t *testing.T) {
			t.Parallel()

			submitKey, err := VideoSubmitReqKey(preset)
			require.NoError(t, err)

			queryKey, err := VideoQueryReqKey(preset)
			require.NoError(t, err)

			require.Equal(t, submitKey, queryKey)
		})
	}
}

func TestValidatePresetCombination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preset   VideoPreset
		hasImage bool
		wantErr  bool
	}{
		// T2V presets
		{"t2v-720-no-image", VideoPresetT2V720, false, false},
		{"t2v-720-with-image", VideoPresetT2V720, true, true},
		{"t2v-1080-no-image", VideoPresetT2V1080, false, false},
		{"t2v-1080-with-image", VideoPresetT2V1080, true, true},
		{"t2v-pro-no-image", VideoPresetT2VPro, false, false},
		{"t2v-pro-with-image", VideoPresetT2VPro, true, true},

		// I2V presets
		{"i2v-first-with-image", VideoPresetI2VFirst, true, false},
		{"i2v-first-no-image", VideoPresetI2VFirst, false, true},
		{"i2v-first-tail-with-image", VideoPresetI2VFirstTail, true, false},
		{"i2v-first-tail-no-image", VideoPresetI2VFirstTail, false, true},
		{"i2v-recamera-with-image", VideoPresetI2VRecamera, true, false},
		{"i2v-recamera-no-image", VideoPresetI2VRecamera, false, true},
		{"i2v-first-pro-with-image", VideoPresetI2VFirstPro, true, false},
		{"i2v-first-pro-no-image", VideoPresetI2VFirstPro, false, true},

		// Unsupported preset
		{"unsupported-preset", VideoPreset("invalid"), false, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePresetCombination(tt.preset, tt.hasImage)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetPresetCapabilities(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name               string
		preset             VideoPreset
		requiresImage      bool
		acceptsImage       bool
		acceptsFrames      bool
		acceptsTemplate    bool
		minImageCount      int
		maxImageCount      int
		defaultFrames      int
		defaultAspectRatio string
	}{
		{
			name:               "t2v-720",
			preset:             VideoPresetT2V720,
			requiresImage:      false,
			acceptsImage:       false,
			acceptsFrames:      true,
			acceptsTemplate:    false,
			minImageCount:      0,
			maxImageCount:      0,
			defaultFrames:      121,
			defaultAspectRatio: "16:9",
		},
		{
			name:               "t2v-1080",
			preset:             VideoPresetT2V1080,
			requiresImage:      false,
			acceptsImage:       false,
			acceptsFrames:      true,
			acceptsTemplate:    false,
			minImageCount:      0,
			maxImageCount:      0,
			defaultFrames:      121,
			defaultAspectRatio: "16:9",
		},
		{
			name:               "t2v-pro",
			preset:             VideoPresetT2VPro,
			requiresImage:      false,
			acceptsImage:       false,
			acceptsFrames:      true,
			acceptsTemplate:    false,
			minImageCount:      0,
			maxImageCount:      0,
			defaultFrames:      121,
			defaultAspectRatio: "16:9",
		},
		{
			name:               "i2v-first",
			preset:             VideoPresetI2VFirst,
			requiresImage:      true,
			acceptsImage:       true,
			acceptsFrames:      false,
			acceptsTemplate:    false,
			minImageCount:      1,
			maxImageCount:      1,
			defaultFrames:      0,
			defaultAspectRatio: "",
		},
		{
			name:               "i2v-first-pro",
			preset:             VideoPresetI2VFirstPro,
			requiresImage:      true,
			acceptsImage:       true,
			acceptsFrames:      false,
			acceptsTemplate:    false,
			minImageCount:      1,
			maxImageCount:      1,
			defaultFrames:      0,
			defaultAspectRatio: "",
		},
		{
			name:               "i2v-first-tail",
			preset:             VideoPresetI2VFirstTail,
			requiresImage:      true,
			acceptsImage:       true,
			acceptsFrames:      false,
			acceptsTemplate:    false,
			minImageCount:      2,
			maxImageCount:      2,
			defaultFrames:      0,
			defaultAspectRatio: "",
		},
		{
			name:               "i2v-recamera",
			preset:             VideoPresetI2VRecamera,
			requiresImage:      true,
			acceptsImage:       true,
			acceptsFrames:      false,
			acceptsTemplate:    true,
			minImageCount:      1,
			maxImageCount:      1,
			defaultFrames:      0,
			defaultAspectRatio: "",
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cap := GetPresetCapabilities(tc.preset)
			require.True(t, cap.Supported)
			require.Equal(t, tc.requiresImage, cap.RequiresImage)
			require.Equal(t, tc.acceptsImage, cap.AcceptsImage)
			require.Equal(t, tc.acceptsFrames, cap.AcceptsFrames)
			require.Equal(t, tc.acceptsTemplate, cap.AcceptsTemplate)
			require.Equal(t, tc.minImageCount, cap.MinImageCount)
			require.Equal(t, tc.maxImageCount, cap.MaxImageCount)
			require.Equal(t, tc.defaultFrames, cap.DefaultFrames)
			require.Equal(t, tc.defaultAspectRatio, cap.DefaultAspectRatio)
		})
	}
}

func TestGetPresetCapabilities_UnknownPreset(t *testing.T) {
	t.Parallel()

	cap := GetPresetCapabilities(VideoPreset("unknown"))
	require.False(t, cap.Supported)
}
