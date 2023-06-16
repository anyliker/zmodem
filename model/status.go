package model

import (
	"fmt"
	"time"
)

type TransferProgress struct {
	Perc      float32
	StartTs   int64
	TotalSize int64
	CurrSize  int64
	Name      string

	LastSize int64 // 用来计算瞬时速度
	LastTs   int64 // 用来计算瞬时速度

	LeftTs int64
	Speed  int
}

func (slf *TransferProgress) Setup(totalSize int64) {
	slf.TotalSize = totalSize
	nowTs := time.Now().Unix()
	slf.StartTs = nowTs
	slf.LastTs = nowTs
}

func (slf *TransferProgress) SetCurrSize(name string, size, totalSize int64) (toUpdate bool) {
	nowTs := time.Now().Unix()
	slf.TotalSize = totalSize
	slf.Name = name
	tsDiff := nowTs - slf.LastTs
	if tsDiff <= 0 {
		return false
	}

	if size > 0 {
		slf.CurrSize = size
		slf.Perc = float32(slf.CurrSize) / float32(slf.TotalSize)

		tsUse := nowTs - slf.StartTs
		totalTsMaybe := float32(tsUse) / slf.Perc

		sizeDiff := size
		if slf.LastSize > 0 {
			sizeDiff = size - slf.LastSize

		}
		if tsDiff > 0 {
			slf.Speed = int(sizeDiff / tsDiff)
			toUpdate = true
		}

		slf.LeftTs = int64(totalTsMaybe) - tsUse
	}
	slf.LastSize = size
	slf.LastTs = nowTs
	return
}

func (slf *TransferProgress) ShowString() string {
	return fmt.Sprintf("%v %13s %13s %13s ETA",
		slf.Name, fmt.Sprintf("%.2f%%", slf.Perc*100), fmt.Sprintf("%v/s", showMSpeed(slf.Speed)), showTime(slf.LeftTs))
}

func (slf *TransferProgress) ShowDoneString() string {
	nowTs := time.Now().Unix()
	tsUse := nowTs - slf.StartTs
	if tsUse == 0 {
		tsUse = 1
	}

	return fmt.Sprintf("%v %13s %13s %13s \r\n",
		slf.Name, "100%", fmt.Sprintf("%v/s", showMSpeed(int(slf.TotalSize/tsUse))), showTime(tsUse))
}

const (
	MB = 1 << 20
	KB = 1 << 10
)

func showMSpeed(in int) string {
	if in > MB {
		return fmt.Sprintf("%.2f MB", float64(in)/float64(MB))
	}
	if in > KB {
		return fmt.Sprintf("%.2f KB", float64(in)/float64(KB))
	}
	return fmt.Sprintf("%d B", in)
}

func showTime(sec int64) string {
	return fmt.Sprintf("%02d:%02d:%02d", sec/3600, sec%3600/60, sec%60)
}
