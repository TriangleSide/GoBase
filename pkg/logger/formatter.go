// Copyright (c) 2024 David Ouellette.
//
// All rights reserved.
//
// This software and its documentation are proprietary information of David Ouellette.
// No part of this software or its documentation may be copied, transferred, reproduced,
// distributed, modified, or disclosed without the prior written permission of David Ouellette.
//
// Unauthorized use of this software is strictly prohibited and may be subject to civil and
// criminal penalties.
//
// By using this software, you agree to abide by the terms specified herein.

package logger

import (
	"github.com/sirupsen/logrus"
)

// UTCFormatter sets the timezone of the log to UTC.
type UTCFormatter struct {
	Next logrus.Formatter
}

// Format sets the timezone of the log to UTC then invokes the next formatter.
func (f *UTCFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	entry.Time = entry.Time.UTC()
	return f.Next.Format(entry)
}
