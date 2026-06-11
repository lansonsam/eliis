package lens

import (
	"errors"

	"github.com/lansonsam/eliis/internal/core/types"
)

// OverrideModel rewrites the IR model before encoding for the output protocol.
type OverrideModel struct {
	Model string
}

func (l OverrideModel) Name() string {
	return "override_model"
}

func (l OverrideModel) Apply(req *types.UnifiedRequest) error {
	if req == nil {
		return errors.New("nil request")
	}
	if l.Model == "" {
		return nil
	}
	req.Model = l.Model
	return nil
}
