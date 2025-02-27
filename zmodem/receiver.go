package zmodem

import (
	"github.com/anyliker/zmodem/byteutil"
	"github.com/anyliker/zmodem/model"
)

func (t *ZModem) handleReceive() {
	if !t.running.CompareAndSwap(false, true) {
		//避免多个线程一起执行
		return
	}
	defer t.running.CompareAndSwap(true, false)

	tp := &model.TransferProgress{}
	tp.Setup(0)
	for true {
		dataFrame, err := t.readFrame()
		if err != nil {
			//解析错误属于正常现象，因为可能一个大数据包被分成两段发过来了，需要等待第二段到位才能够正常解析
			return
		}
		//println("解析到接收帧")
		//println(dataFrame.ToString() + "\n")
		switch dataFrame.frameType {
		case ZRQINIT:
			err = t.SendFrame(NewHexFrame(ZRINIT, DEFAULT_HEADER_DATA))
			if err != nil {
				return
			}
			break
		case ZFILE:
			packet, err := t.readSubPacket(dataFrame.encoding)
			if err != nil {
				//子包读取错误直接抛异常
				t.close()
				return
			}
			file, err := parseZModemFile(packet.data)
			if err != nil {
				return
			}
			t.consumer.OnCheckDownload(&file)
			if file.isSkip {
				t.lastDownloadFile = nil
				//是否跳过
				err = t.SendFrame(NewHexFrame(ZSKIP, DEFAULT_HEADER_DATA))
				if err != nil {
					return
				}
				//文件全部跳过了
				t.close()
				return
			} else {
				//不跳过
				t.lastDownloadFile = &file
				err = t.SendFrame(NewHexFrame(ZRPOS, DEFAULT_HEADER_DATA))
				if err != nil {
					return
				}
			}
			break
		case ZDATA:
			//文件传输中
			isEnd := false
			if t.lastDownloadFile == nil {
				t.close()
				return
			}
			buf := byteutil.NewBlockReadWriterBuf(make([]byte, 0, 0x2fff), int64(t.lastDownloadFile.Size))
			t.lastDownloadFile.downloadStream = buf
			go func() {
				err := t.consumer.OnDownload(t.lastDownloadFile, buf)
				if err != nil {
					t.close()
					return
				}
			}()

			isErrorEnd := false
			currSize := int64(0)
			for !isEnd {
				//子包读取错误直接抛异常
				packet, err := t.readSubPacket(dataFrame.encoding)
				if err != nil {
					//子包读取错误直接抛异常
					t.close()
					_ = buf.Close()
					break
				}
				_, err = buf.Write(packet.data)
				if err != nil {
					if !isErrorEnd {
						t.sendClose()
					}
					isErrorEnd = true
				}

				currSize += int64(len(packet.data))
				// 回调进度
				if tp.SetCurrSize(t.lastDownloadFile.Filename, currSize, int64(t.lastDownloadFile.Size)) {
					t.consumer.OnTransferring(tp)
				}

				isEnd = packet.isEnd
			}
			if isErrorEnd {
				_ = buf.Close()
				_, _ = t.consumer.Writer.Write([]byte("\n"))
			} else if isEnd {
				_ = buf.Close()

				tp.Name = t.lastDownloadFile.Filename
				tp.TotalSize = int64(t.lastDownloadFile.Size)
				t.consumer.OnTransferDone(t, tp)
			}
		case ZEOF:
			//文件传输完毕
			err = t.SendFrame(NewHexFrame(ZRINIT, DEFAULT_HEADER_DATA))
			if err != nil {
				return
			}
			break
		case ZFIN:
			//完成
			_ = t.SendFrame(NewHexFrame(ZFIN, DEFAULT_HEADER_DATA))
			t.release()
			return
		default:
			t.close()
			return
		}

	}
	return
}
