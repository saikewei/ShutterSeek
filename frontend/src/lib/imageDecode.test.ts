import { describe, expect, it } from 'vitest'
import { extractEmbeddedJPEG } from './imageDecode'

// 构造一个结构合法的 JPEG：SOI + SOF0 + SOS + 熵数据 + EOI
// pad 控制熵数据长度，用于制造“更大的 JPEG”。
function makeJPEG(seed: number, pad = 0): Uint8Array {
  const len = 28 + pad
  const buf = new Uint8Array(len)
  buf[0] = 0xff
  buf[1] = 0xd8 // SOI
  // SOF0：FF C0，长度 11（含长度字节），精度 8，高 4，宽 4，1 分量
  buf[2] = 0xff
  buf[3] = 0xc0
  buf[4] = 0x00
  buf[5] = 0x0b
  buf[6] = 0x08
  buf[7] = 0x00
  buf[8] = 0x04
  buf[9] = 0x00
  buf[10] = 0x04
  buf[11] = 0x01
  buf[12] = 0x01
  buf[13] = 0x11
  buf[14] = 0x00
  // SOS：FF DA，长度 8，1 分量，频谱 0x00 0x3f 0x00
  buf[15] = 0xff
  buf[16] = 0xda
  buf[17] = 0x00
  buf[18] = 0x08
  buf[19] = 0x01
  buf[20] = 0x01
  buf[21] = 0x00
  buf[22] = 0x00
  buf[23] = 0x3f
  buf[24] = 0x00
  for (let k = 0; k <= pad; k++) buf[25 + k] = seed
  buf[26 + pad] = 0xff
  buf[27 + pad] = 0xd9 // EOI
  return buf
}

describe('extractEmbeddedJPEG', () => {
  it('从二进制里找到最大的合法 JPEG', () => {
    const small = makeJPEG(1)
    const bigger = makeJPEG(2, 4)
    const data = new Uint8Array(bigger.length + small.length)
    data.set(bigger, 0)
    data.set(small, bigger.length)
    const got = extractEmbeddedJPEG(data)
    expect(got).not.toBeNull()
    expect(got!.length).toBe(bigger.length)
  })

  it('拒绝只有 APP 段的伪 JPEG', () => {
    const data = new Uint8Array([0xff, 0xd8, 0xff, 0xe1, 0, 4, 1, 2, 0xff, 0xd9])
    expect(extractEmbeddedJPEG(data)).toBeNull()
  })
})
