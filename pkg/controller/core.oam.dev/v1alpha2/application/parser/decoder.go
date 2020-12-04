package parser

import "encoding/json"

// DecodeJSONMarshaler decode json.Marshaler to map[string]interface
func DecodeJSONMarshaler(i json.Marshaler) (map[string]interface{}, error) {
	jst, err := i.MarshalJSON()
	if err != nil {
		return nil, err
	}
	ret := map[string]interface{}{}
	if err := json.Unmarshal(jst, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}
