image compress 12

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