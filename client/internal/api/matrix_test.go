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

	require.Equal(t, "jimeng_t2v_v30_720p", ReqKeyJimengT2VV30_720p)
	require.Equal(t, "jimeng_t2v_v30_1080p", ReqKeyJimengT2VV30_1080p)
	require.Equal(t, "jimeng_i2v_first_v30_1080", ReqKeyJimengI2VFirstV30_1080)
	require.Equal(t, "jimeng_i2v_first_tail_v30_1080", ReqKeyJimengI2VFirstTailV30_1080)
	require.Equal(t, "jimeng_i2v_recamera_v30", ReqKeyJimengI2VRecameraV30)
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
