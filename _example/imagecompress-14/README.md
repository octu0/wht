image compress 14

- YCbCr 4:2:0
- Multi-Resolution Discrete Wavelet Transform (LeGall 5/3)
  - Global DWT (Full frame transform, no block artifacts)
  - 3-Layer Progressive Encoding
    - Layer 0: Thumbnail (Base LL band)
    - Layer 1: Medium Quality (Adds HL, LH, HH of level 1)
    - Layer 2: High Quality (Adds HL, LH, HH of level 0)
- Content-Adaptive Bit-shift Quantization
- Unified Block RLE / Rice coding
  - Signed integer mapping (Zigzag mapping) for efficient Rice coding
- Virtual Buffer based CBR (Rate Controller)
- Intra Prediction (DC Prediction) for LL band

## Output
The encoder generates three separate layer files:
- `out_layer0.png`: Decoded from Layer 0 (Thumbnail quality)
- `out_layer1.png`: Decoded from Layer 0 + 1 (Medium quality)
- `out_layer2.png`: Decoded from Layer 0 + 1 + 2 (Full quality)

## Benchmark
Multi-Resolution Discrete Wavelet Transform (LeGall 5/3) vs JPEG (Multi-Resolution View).

### Method
- **L (Large)**: Full resolution (Original size)
- **M (Mid)**: 1/2 resolution (Downsampled by Nearest Neighbor)
- **S (Small)**: 1/4 resolution (Downsampled by Nearest Neighbor)

JPEG images were also downsampled using the same Nearest Neighbor method for fair comparison at lower resolutions.

### Result (JPEG)
| Quality | Size (KB) | Layer | PSNR (Avg) | SSIM (Avg) | MS-SSIM (Avg) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **50** | 13.65 | **L** | 31.96 | 0.8517 | 0.9488 |
| | | M | 32.00 | 0.8849 | 0.9649 |
| | | S | 31.95 | 0.9007 | 0.9744 |
| **70** | 18.38 | **L** | 33.32 | 0.8804 | 0.9614 |
| | | M | 33.46 | 0.9114 | 0.9733 |
| | | S | 33.79 | 0.9286 | 0.9816 |
| **90** | 34.17 | **L** | 37.58 | 0.9377 | 0.9821 |
| | | M | 37.62 | 0.9546 | 0.9866 |
| | | S | 38.02 | 0.9644 | 0.9909 |

### Result (My Codec)
| Bitrate (kbps) | Size (KB) | Layer | PSNR (Avg) | SSIM (Avg) | MS-SSIM (Avg) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **100** | 36.98 | **L** | 25.00 | 0.7293 | 0.8245 |
| | | M | 25.10 | 0.7473 | 0.8717 |
| | | S | 25.10 | 0.7709 | 0.9121 |
| **400** | 78.52 | **L** | 26.42 | 0.8146 | 0.8660 |
| | | M | 26.39 | 0.8187 | 0.8941 |
| | | S | 25.10 | 0.7709 | 0.9121 |
| **1900** | 105.77 | **L** | 26.92 | 0.8444 | 0.8757 |
| | | M | 26.39 | 0.8187 | 0.8941 |
| | | S | 25.10 | 0.7709 | 0.9121 |
