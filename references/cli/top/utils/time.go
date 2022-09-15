/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// UnknownTime represent the time can't successfully be formatted
const UnknownTime = "UNKNOWN"

// TimeFormat format time data of `time.Duration` type to string type
func TimeFormat(t time.Duration) string {
	timeStr := t.String()
	// time < 1s
	if t.Seconds() < 1 {
		return timeStr
	}
	// cut in the place of decimal point
	removeDecimal := strings.Split(timeStr, ".")
	if len(removeDecimal) > 1 {
		removeDecimal[0] += "s"
	}
	// cut in the place of 'h'
	convertHourToDay := strings.Split(removeDecimal[0], "h")
	if len(convertHourToDay) > 1 {
		// hour num
		hour, err := strconv.Atoi(convertHourToDay[0])
		if err != nil {
			return UnknownTime
		}
		if hour > 24 {
			return fmt.Sprintf("%dd%dh%s", hour/24, hour%24, convertHourToDay[1])
		}
	}
	return removeDecimal[0]

}
