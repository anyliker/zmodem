package zmodem

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/anyliker/zmodem/myioutil"
	"io"
)

func (t *ZModem) handleSend() {
	if !t.running.CompareAndSwap(false, true) {
		//避免多个线程一起执行
		return
	}
	defer t.running.CompareAndSwap(true, false)

	for true {
		dataFrame, err := t.readFrame()
		if err != nil {
			//解析错误属于正常现象，因为可能一个大数据包被分成两段发过来了，需要等待第二段到位才能够正常解析
			return
		}
		fmt.Println("接收...", dataFrame.frameType)

		switch dataFrame.frameType {
		case ZRINIT:
			if t.lastUploadFile != nil {
				// 上一个文件已完成
				currFile := t.lastUploadFile.GetCurrFile()
				if currFile != nil {
					t.consumer.OnTransferDone(t, currFile.tp)
				}

				// 所有文件有处理完了
				if !t.lastUploadFile.HasMoreFile() {
					err = t.SendFrame(NewHexFrame(ZFIN, DEFAULT_HEADER_DATA))
					break
				}
			} else {
				zmodemFile := t.consumer.OnUpload()
				if zmodemFile == nil {
					t.close()
					return
				}

				t.lastUploadFile = zmodemFile
			}

			// 下一个文件,  默认从 -1 开始
			t.lastUploadFile.CurrIdx++
			err = t.SendFrame(NewBinFrame(ZFILE, DEFAULT_HEADER_DATA))
			if err != nil {
				return
			}

			currFile := t.lastUploadFile.GetCurrFile()
			currFile.tp.Setup(int64(currFile.Size))

			err = t.sendSubPacket(newSubPacket(ZCRCW, currFile.marshal()), ZBIN, true)
			if t.lastUploadFile == nil {
				t.close()
				return
			}

			break
		case ZRPOS:
			if t.lastUploadFile == nil {
				t.close()
				return
			}
			currFile := t.lastUploadFile.GetCurrFile()
			size := currFile.Size

			// 支持中断
			if t.lastUploadFile.forceCancel {
				t.close()
				return
			}

			//发送文件内容
			err = t.SendFrame(NewBinFrame(ZDATA, DEFAULT_HEADER_DATA))
			if err != nil {
				t.close()
				return
			}

			tp := currFile.tp
			//8k一个包发送
			_, err = myioutil.CopyFixedSize(myioutil.WriteFunc(func(p []byte) (n int, err error) {
				n = len(p)
				isEnd := currFile.writeCount+n >= size
				currFile.writeCount += n

				// 支持中断
				if t.lastUploadFile.forceCancel {
					return 0, errors.New("取消")
				}

				// 回调进度
				if tp.SetCurrSize(currFile.Filename, int64(currFile.writeCount), int64(size)) {
					t.consumer.OnTransferring(tp)
				}
				if isEnd {
					if err == nil || err == io.EOF {
						//正常读取完毕
						err = t.sendSubPacket(newSubPacket(ZCRCE, p), ZBIN, false)
						if err != nil {
							return
						}
						sizeBytes := make([]byte, 4)
						binary.LittleEndian.PutUint32(sizeBytes, uint32(size))
						err = t.SendFrame(NewBinFrame(ZEOF, sizeBytes))

						//if t.lastUploadFile.HasMoreFile() {
						//	err = t.SendFrame(NewHexFrame(ZRQINIT, DEFAULT_HEADER_DATA))
						//}

						//t.sendFileEOF = true
					}
				} else {
					err = t.sendSubPacket(newSubPacket(ZCRCG, p), ZBIN, false)
					if err != nil {
						fmt.Println(err)
					}
				}
				return
			}), currFile.uploadFile, 8192)

			if err != nil && err != io.EOF {
				//非正常关闭
				t.close()
				return
			}
		case ZSKIP:
			//跳过
			if t.lastUploadFile == nil {
				t.close()
				return
			}

			t.consumer.OnUploadSkip(t.lastUploadFile.GetCurrFile())

			if !t.lastUploadFile.HasMoreFile() {
				err = t.SendFrame(NewHexFrame(ZFIN, DEFAULT_HEADER_DATA))

				t.lastUploadFile.Close()
				t.lastUploadFile = nil
			} else {
				// 直接开始下一个文件
				t.lastUploadFile.CurrIdx++
				err = t.SendFrame(NewBinFrame(ZFILE, DEFAULT_HEADER_DATA))
				if err != nil {
					return
				}

				currFile := t.lastUploadFile.GetCurrFile()
				currFile.tp.Setup(int64(currFile.Size))

				err = t.sendSubPacket(newSubPacket(ZCRCW, currFile.marshal()), ZBIN, true)
				if t.lastUploadFile == nil {
					t.close()
					return
				}
			}
		case ZFIN:
			//完成
			_, err = t.consumer.Writer.Write([]byte{'O', 'O'})
			if err != nil {
				fmt.Println(err)
			}

			//tp := t.lastUploadFile.
			t.release()

			//t.consumer.OnTransferDone(t, tp)
			return
		default:
			// 看是不是有关闭
			t.close()
			return
		}

	}
	return
}
