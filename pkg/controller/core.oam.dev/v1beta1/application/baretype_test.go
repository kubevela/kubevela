package application

import (
	"testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/stretchr/testify/require"
)

func makeTestApp(compType string, annotations map[string]string) *v1beta1.Application {
	app := &v1beta1.Application{}
	app.Annotations = annotations
	app.Spec.Components = []common.ApplicationComponent{
		{Name: "my-bucket", Type: compType},
	}
	app.Status.LatestRevision = &common.Revision{RevisionHash: "oldhash"}
	return app
}

func makeTestRev(cueTemplate string) *v1beta1.ApplicationRevision {
	rev := &v1beta1.ApplicationRevision{}
	rev.Spec.ComponentDefinitions = map[string]*v1beta1.ComponentDefinition{
		"aws-s3-v1": {
			Spec: v1beta1.ComponentDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: cueTemplate},
				},
			},
		},
	}
	return rev
}

func TestHasBareTypeComponents(t *testing.T) {
	cases := []struct {
		name  string
		types []string
		want  bool
	}{
		{"all bare", []string{"aws-s3-v1", "webservice"}, true},
		{"all versioned", []string{"aws-s3-v1@v2", "webservice@v1"}, false},
		{"mixed", []string{"aws-s3-v1@v2", "webservice"}, true},
		{"empty", []string{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &v1beta1.Application{}
			for _, typ := range tc.types {
				app.Spec.Components = append(app.Spec.Components, common.ApplicationComponent{Type: typ})
			}
			require.Equal(t, tc.want, hasBareTypeComponents(app))
		})
	}
}

func TestBugRegression(t *testing.T) {
	t.Run("bare_type_same_def_no_annotation", func(t *testing.T) {
		app := makeTestApp("aws-s3-v1", nil)
		old := makeTestRev("schema-v12")
		cur := makeTestRev("schema-v12")
		require.True(t, hasBareTypeComponents(app))
		require.True(t, DeepEqualRevision(old, cur), "same def → no new revision needed")
	})

	t.Run("bare_type_def_changed_no_annotation_IS_THE_BUG", func(t *testing.T) {
		app := makeTestApp("aws-s3-v1", nil)
		old := makeTestRev("schema-v12")
		cur := makeTestRev("schema-v13")
		require.True(t, hasBareTypeComponents(app))
		require.True(t, deepEqualAppInRevision(old, cur), "PRE-FIX: shallow compare misses def change")
		require.False(t, DeepEqualRevision(old, cur), "POST-FIX: deep compare catches def change")
	})

	t.Run("versioned_type_not_affected", func(t *testing.T) {
		app := makeTestApp("aws-s3-v1@v2", nil)
		require.False(t, hasBareTypeComponents(app), "versioned type must not be treated as bare")
	})

	t.Run("bare_type_with_autoUpdate_still_works", func(t *testing.T) {
		app := makeTestApp("aws-s3-v1", map[string]string{oam.AnnotationAutoUpdate: "true"})
		old := makeTestRev("schema-v12")
		cur := makeTestRev("schema-v13")
		require.True(t, hasBareTypeComponents(app))
		require.False(t, DeepEqualRevision(old, cur), "def change must trigger new revision")
	})
}
