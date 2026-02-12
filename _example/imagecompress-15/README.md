# Image Compress 15

Multi-resolution DWT-based image compression codec.  

## Technical Stack

- **Color Space**: YCbCr 4:2:0
- **Transform**: Multi-Resolution Discrete Wavelet Transform (LeGall 5/3) 2-level 2D block transform
  - Macroblock DWT (no block artifacts)
  - 3-Layer Progressive Encoding
    - Layer 0: Thumbnail (Base LL band)
    - Layer 1: Medium Quality (Adds HL, LH, HH of level 1)
    - Layer 2: High Quality (Adds HL, LH, HH of level 0)
- **Quantization**: Content-Adaptive Bit-shift Quantization
  - Flatness detection using HH subband analysis
- **Entropy Coding**: Zero-run Rice coding
  - RLE zero-run cap (maxVal=64) for stability
- **Rate Control**: Progress-based CBR Rate Controller
  - Shared `RateController` across all layers
  - Dynamic `baseShift` adjustment via overshoot ratio against `targetBitsProgress`
- **Prediction**: Intra Prediction
- **Multi-Resolution**: 3-layer structure — Layer0 (1/4) → Layer1 (1/2) → Layer2 (1/1)

## Usage

### Encode / Decode

```bash
# Default: 100kbps
go run .

# Specify bitrate
go run . -bitrate 200
```

### Benchmark

Run quality comparison benchmark against JPEG:

```bash
go run . -benchmark
```

## Benchmark Results

### JPEG (by Quality)

| Quality | Size | PSNR(L) | SSIM(L) | MS-SSIM(L) |
|---------|------|---------|---------|------------|
| 50 | 13.65KB | 31.96 | 0.8517 | 0.9488 |
| 60 | 15.48KB | 32.51 | 0.8638 | 0.9543 |
| 70 | 18.38KB | 33.32 | 0.8804 | 0.9614 |
| 80 | 23.10KB | 34.62 | 0.9012 | 0.9695 |
| 90 | 34.17KB | 37.58 | 0.9377 | 0.9821 |
| 100 | 89.25KB | 58.05 | 0.9994 | 0.9998 |

### Custom Codec (by Bitrate)

| Bitrate | Size | PSNR(L) | SSIM(L) | MS-SSIM(L) |
|---------|------|---------|---------|------------|
| 100k | 14.21KB | 25.72 | 0.7112 | 0.7866 |
| 200k | 32.98KB | 35.30 | 0.9412 | 0.9723 |
| 300k | 33.06KB | 35.32 | 0.9413 | 0.9723 |
| 400k | 33.12KB | 35.32 | 0.9414 | 0.9723 |
| 500k | 33.12KB | 35.32 | 0.9414 | 0.9723 |

- **Comparison**: Custom Codec at 200k (32.98KB) achieves PSNR +1.98dB / SSIM +0.06 over JPEG Q=70 (18.38KB), but at ~1.8x the file size
- **Feature**: Multi-resolution progressive decoding (thumbnail → medium → full quality)
- **Saturation**: Above 200kbps, entropy coding reaches its floor — no further change in size or quality
