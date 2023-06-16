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

		switch dataFrame.frameType {
		case ZRINIT:
			if t.lastUploadFile != nil {
				//传输完成
				if t.sendFileEOF {
					err = t.SendFrame(NewHexFrame(ZFIN, DEFAULT_HEADER_DATA))
					t.sendFileEOF = false

					file := t.lastUploadFile
					t.lastUploadFile.tp.Name = file.Filename
					t.lastUploadFile.tp.TotalSize = int64(file.Size)
				}
				break
			}

			zmodemFile := t.consumer.OnUpload()
			if zmodemFile == nil {
				t.close()
				return
			}
			err = t.SendFrame(NewBinFrame(ZFILE, DEFAULT_HEADER_DATA))
			if err != nil {
				return
			}
			t.lastUploadFile = zmodemFile
			err = t.sendSubPacket(newSubPacket(ZCRCW, zmodemFile.marshal()), ZBIN, true)
			if zmodemFile == nil {
				t.close()
				return
			}

			t.lastUploadFile.tp.Setup(int64(t.lastUploadFile.Size))
			break
		case ZRPOS:
			if t.lastUploadFile == nil {
				t.close()
				return
			}
			size := t.lastUploadFile.Size

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

			tp := t.lastUploadFile.tp
			//8k一个包发送
			_, err = myioutil.CopyFixedSize(myioutil.WriteFunc(func(p []byte) (n int, err error) {
				n = len(p)
				isEnd := t.lastUploadFile.writeCount+n >= size
				t.lastUploadFile.writeCount += n

				// 支持中断
				if t.lastUploadFile.forceCancel {
					return 0, errors.New("取消")
				}

				// 回调进度
				if tp.SetCurrSize(t.lastUploadFile.Filename, int64(t.lastUploadFile.writeCount), int64(size)) {
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
						t.sendFileEOF = true
					}
				} else {
					err = t.sendSubPacket(newSubPacket(ZCRCG, p), ZBIN, false)
					if err != nil {
						fmt.Println(err)
					}
				}
				return
			}), t.lastUploadFile.uploadFile, 8192)

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
			t.consumer.OnUploadSkip(t.lastUploadFile)
			err = t.SendFrame(NewHexFrame(ZFIN, DEFAULT_HEADER_DATA))
			_ = t.lastUploadFile.uploadFile.Close()
			t.lastDownloadFile = nil
		case ZFIN:
			//完成
			_, err = t.consumer.Writer.Write([]byte{'O', 'O'})
			if err != nil {
				fmt.Println(err)
			}

			tp := t.lastUploadFile.tp
			t.release()

			t.consumer.OnTransferDone(t, tp)
			return
		default:
			// 看是不是有关闭
			t.close()
			return
		}

	}
	return
}
