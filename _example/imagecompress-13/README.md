image compress 13

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
| **75** | 20.25 | **L** | 33.85 | 0.8897 | 0.9650 |
| | | M | 33.94 | 0.9178 | 0.9751 |
| | | S | 34.23 | 0.9338 | 0.9832 |
| **90** | 34.17 | **L** | 37.58 | 0.9377 | 0.9821 |
| | | M | 37.62 | 0.9546 | 0.9866 |
| | | S | 38.02 | 0.9644 | 0.9909 |

### Result (My Codec)
| Bitrate (kbps) | Size (KB) | Layer | PSNR (Avg) | SSIM (Avg) | MS-SSIM (Avg) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **100** | 39.10 | **L** | 28.30 | 0.7626 | 0.9325 |
| | | M | 28.50 | 0.8025 | 0.9325 |
| | | S | 28.79 | 0.8496 | 0.9610 |
| **400** | 80.63 | **L** | 30.60 | 0.8492 | 0.9288 |
| | | M | 30.64 | 0.8704 | 0.9511 |
| | | S | 28.79 | 0.8496 | 0.9610 |
| **1900** | 107.88 | **L** | 31.66 | 0.8777 | 0.9381 |
| | | M | 30.64 | 0.8704 | 0.9511 |
| | | S | 28.79 | 0.8496 | 0.9610 |
