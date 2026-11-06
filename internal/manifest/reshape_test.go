package manifest

import (
	"os"
	"reflect"
	"testing"
)

func TestFromObjectListFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/object-list.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := FromObjectList(raw)
	if err != nil {
		t.Fatal(err)
	}
	cube := DefaultEntry("cube-1", "cube")
	cube.ScaleCustom = []float64{2, 2, 2}
	cube.ExternalID = stringPtr("ext-42")
	lantern2 := DefaultEntry("lantern-2", "Lantern")
	lantern2.InitialScale = []float64{1.5, 1.5, 1.5}
	want := &Manifest{Objects: []Entry{cube, DefaultEntry("lantern-1", "Lantern"), lantern2}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("FromObjectList =\n%+v\nwant\n%+v", m, want)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("reshaped manifest fails Validate: %v", err)
	}
}

func TestFromObjectListOptionalFields(t *testing.T) {
	raw := []byte(`[{"sdkId":"a","name":null,"meshName":"mesh-a","isController":false,"scale":2.5,
		"scaleCustomX":null,"scaleCustomY":null,"scaleCustomZ":null,
		"initialPositionX":1,"initialPositionY":2,"initialPositionZ":3,
		"initialRotationX":0,"initialRotationY":0,"initialRotationZ":0,"initialRotationW":1,
		"dynamicFileType":"gltf","externalId":null}]`)
	m, err := FromObjectList(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := Entry{
		ID: "a", Mesh: "mesh-a", Name: "mesh-a",
		InitialPosition: []float64{1, 2, 3},
		InitialRotation: []float64{0, 0, 0, 1},
		IsController:    boolPtr(false),
		Scale:           floatPtr(2.5),
		FileType:        stringPtr("gltf"),
	}
	if len(m.Objects) != 1 || !reflect.DeepEqual(m.Objects[0], want) {
		t.Errorf("FromObjectList = %+v, want %+v", m.Objects, want)
	}
}

func TestFromObjectListEmpty(t *testing.T) {
	m, err := FromObjectList([]byte(" []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.Objects == nil || len(m.Objects) != 0 {
		t.Errorf("FromObjectList([]) = %+v, want an empty manifest", m)
	}
}

func TestFromObjectListRejects(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		parts []string
	}{
		{"partial null rotation", `[{"sdkId":"a","meshName":"m","initialRotationX":0,"initialRotationY":0,"initialRotationZ":0,"initialRotationW":null}]`, []string{`"a"`, "initialRotationW"}},
		{"partial initialScale", `[{"sdkId":"b","meshName":"m","initialScaleX":1,"initialScaleY":null,"initialScaleZ":1}]`, []string{`"b"`, "initialScaleY"}},
		{"null sdkId", `[{"sdkId":null,"meshName":"m"}]`, []string{"sdkId"}},
		{"empty sdkId", `[{"sdkId":"","meshName":"m"}]`, []string{"sdkId"}},
		{"missing meshName", `[{"sdkId":"c"}]`, []string{`"c"`, "meshName"}},
		{"not an array", `{"objects":[]}`, []string{"array"}},
		{"null", `null`, []string{"array"}},
		{"malformed", `[`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromObjectList([]byte(tc.raw))
			wantErrNaming(t, err, tc.parts...)
		})
	}
}
