package payment

import qrcode "github.com/skip2/go-qrcode"

// GenerateQRCode 把内容（如 USDT 收款地址）编码为 PNG 二维码字节。
// size 为图片边长像素（如 256）。
func GenerateQRCode(content string, size int) ([]byte, error) {
	if size <= 0 {
		size = 256
	}
	return qrcode.Encode(content, qrcode.Medium, size)
}
