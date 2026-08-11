package features_test

import (
	"context"
	"testing"

	"github.com/saifsilver/goplusplus/features"
)

func TestFeaturesManager(t *testing.T) {
	mgr := features.NewManager()
	ctx := context.Background()

	if !mgr.IsEnabled(ctx, "new_checkout_v2") {
		t.Errorf("expected 'new_checkout_v2' to be enabled")
	}

	if mgr.IsEnabled(ctx, "ai_recommendations") {
		t.Errorf("expected 'ai_recommendations' to be disabled")
	}

	if mgr.IsEnabled(ctx, "non_existent_feature") {
		t.Errorf("expected 'non_existent_feature' to be disabled")
	}
}
