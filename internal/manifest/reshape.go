package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// listObject is one element of the gateway's GET /versions/{versionId}/objects array.
type listObject struct {
	SDKID            *string  `json:"sdkId"`
	Name             *string  `json:"name"`
	MeshName         *string  `json:"meshName"`
	IsController     *bool    `json:"isController"`
	Scale            *float64 `json:"scale"`
	ScaleCustomX     *float64 `json:"scaleCustomX"`
	ScaleCustomY     *float64 `json:"scaleCustomY"`
	ScaleCustomZ     *float64 `json:"scaleCustomZ"`
	InitialPositionX *float64 `json:"initialPositionX"`
	InitialPositionY *float64 `json:"initialPositionY"`
	InitialPositionZ *float64 `json:"initialPositionZ"`
	InitialRotationX *float64 `json:"initialRotationX"`
	InitialRotationY *float64 `json:"initialRotationY"`
	InitialRotationZ *float64 `json:"initialRotationZ"`
	InitialRotationW *float64 `json:"initialRotationW"`
	InitialScaleX    *float64 `json:"initialScaleX"`
	InitialScaleY    *float64 `json:"initialScaleY"`
	InitialScaleZ    *float64 `json:"initialScaleZ"`
	ExternalID       *string  `json:"externalId"`
	DynamicFileType  *string  `json:"dynamicFileType"`
}

// FromObjectList reshapes the gateway's object list into a manifest, keeping
// the input order.
func FromObjectList(raw []byte) (*Manifest, error) {
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errors.New("parse object list: top level is not a JSON array")
	}
	var list []listObject
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parse object list: %w", err)
	}
	m := &Manifest{Objects: make([]Entry, 0, len(list))}
	for i, o := range list {
		e, err := o.entry()
		if err != nil {
			return nil, fmt.Errorf("parse object list: element %d: %w", i, err)
		}
		m.Objects = append(m.Objects, e)
	}
	return m, nil
}

func (o listObject) entry() (Entry, error) {
	if o.SDKID == nil || *o.SDKID == "" {
		return Entry{}, errors.New("sdkId is missing")
	}
	id := *o.SDKID
	if o.MeshName == nil || *o.MeshName == "" {
		return Entry{}, fmt.Errorf("object %q: meshName is missing", id)
	}
	e := Entry{
		ID:           id,
		Mesh:         *o.MeshName,
		Name:         *o.MeshName,
		IsController: o.IsController,
		Scale:        o.Scale,
		ExternalID:   o.ExternalID,
		FileType:     o.DynamicFileType,
	}
	if o.Name != nil {
		e.Name = *o.Name
	}
	var err error
	if e.ScaleCustom, err = components(id, "scaleCustom", "XYZ", o.ScaleCustomX, o.ScaleCustomY, o.ScaleCustomZ); err != nil {
		return Entry{}, err
	}
	if e.InitialPosition, err = components(id, "initialPosition", "XYZ", o.InitialPositionX, o.InitialPositionY, o.InitialPositionZ); err != nil {
		return Entry{}, err
	}
	if e.InitialRotation, err = components(id, "initialRotation", "XYZW", o.InitialRotationX, o.InitialRotationY, o.InitialRotationZ, o.InitialRotationW); err != nil {
		return Entry{}, err
	}
	if e.InitialScale, err = components(id, "initialScale", "XYZ", o.InitialScaleX, o.InitialScaleY, o.InitialScaleZ); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// components returns nil when every value is null and an error when only some are.
func components(id, field, suffixes string, values ...*float64) ([]float64, error) {
	set := 0
	for _, v := range values {
		if v != nil {
			set++
		}
	}
	if set == 0 {
		return nil, nil
	}
	out := make([]float64, len(values))
	for i, v := range values {
		if v == nil {
			return nil, fmt.Errorf("object %q: %s%c is null while other %s components are set", id, field, suffixes[i], field)
		}
		out[i] = *v
	}
	return out, nil
}
