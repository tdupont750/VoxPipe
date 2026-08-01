using System.Text;
using PvWhisper.Audio;

namespace PvWhisper.Tests;

public class WavConverterTests
{
    private const int HeaderBytes = 44;
    private const int SampleRate = 16000;

    private static readonly WavConverter Converter = new WavConverter();

    [Fact]
    public void CreateWavFromPcm16_WritesCanonicalHeader()
    {
        // Arrange
        var samples = new short[] { 1, -1, 32767, -32768 };

        // Act
        using var stream = Converter.CreateWavFromPcm16(samples, SampleRate);
        var bytes = stream.ToArray();

        // Assert
        var expectedData = samples.Length * 2;
        Assert.Equal(HeaderBytes + expectedData, bytes.Length);

        Assert.Equal("RIFF", Encoding.ASCII.GetString(bytes, 0, 4));
        Assert.Equal(4 + 24 + 8 + expectedData, BitConverter.ToInt32(bytes, 4));
        Assert.Equal("WAVE", Encoding.ASCII.GetString(bytes, 8, 4));

        Assert.Equal("fmt ", Encoding.ASCII.GetString(bytes, 12, 4));
        Assert.Equal(16, BitConverter.ToInt32(bytes, 16));      // subchunk1 size
        Assert.Equal(1, BitConverter.ToInt16(bytes, 20));       // PCM
        Assert.Equal(1, BitConverter.ToInt16(bytes, 22));       // mono
        Assert.Equal(SampleRate, BitConverter.ToInt32(bytes, 24));
        Assert.Equal(SampleRate * 2, BitConverter.ToInt32(bytes, 28)); // byte rate
        Assert.Equal(2, BitConverter.ToInt16(bytes, 32));       // block align
        Assert.Equal(16, BitConverter.ToInt16(bytes, 34));      // bits per sample

        Assert.Equal("data", Encoding.ASCII.GetString(bytes, 36, 4));
        Assert.Equal(expectedData, BitConverter.ToInt32(bytes, 40));
    }

    [Fact]
    public void CreateWavFromPcm16_WritesSamplesLittleEndian()
    {
        // Arrange
        var samples = new short[] { 1, -1, 32767, -32768, 0x1234 };

        // Act
        using var stream = Converter.CreateWavFromPcm16(samples, SampleRate);
        var bytes = stream.ToArray();

        // Assert
        for (var i = 0; i < samples.Length; i++)
        {
            var actual = BitConverter.ToInt16(bytes, HeaderBytes + i * 2);
            Assert.Equal(samples[i], actual);
        }

        // Spot-check the raw byte order so this stays a real endianness assertion
        Assert.Equal(0x34, bytes[HeaderBytes + 8]);
        Assert.Equal(0x12, bytes[HeaderBytes + 9]);
    }

    [Fact]
    public void CreateWavFromPcm16_RewindsStreamForReading()
    {
        // Arrange
        var samples = new short[] { 5, 6, 7 };

        // Act
        using var stream = Converter.CreateWavFromPcm16(samples, SampleRate);

        // Assert
        Assert.Equal(0, stream.Position);
        Assert.Equal(HeaderBytes + samples.Length * 2, stream.Length);
    }

    [Fact]
    public void CreateWavFromPcm16_EmptySamples_WritesHeaderOnly()
    {
        // Act
        using var stream = Converter.CreateWavFromPcm16(Array.Empty<short>(), SampleRate);
        var bytes = stream.ToArray();

        // Assert
        Assert.Equal(HeaderBytes, bytes.Length);
        Assert.Equal(0, BitConverter.ToInt32(bytes, 40));   // data size
        Assert.Equal(36, BitConverter.ToInt32(bytes, 4));   // chunk size
    }

    [Fact]
    public void CreateWavFromPcm16_LargeBuffer_MatchesPerSampleEncoding()
    {
        // Arrange: a full 60s capture, the case the exact-capacity change targets
        var samples = new short[SampleRate * 60];
        for (var i = 0; i < samples.Length; i++)
            samples[i] = (short)(i % 1000 - 500);

        // Act
        using var stream = Converter.CreateWavFromPcm16(samples, SampleRate);
        var bytes = stream.ToArray();

        // Assert: byte-for-byte identical to writing each sample individually
        var expected = new byte[samples.Length * 2];
        for (var i = 0; i < samples.Length; i++)
        {
            var pair = BitConverter.GetBytes(samples[i]);
            expected[i * 2] = pair[0];
            expected[i * 2 + 1] = pair[1];
        }

        Assert.Equal(expected, bytes.AsSpan(HeaderBytes).ToArray());
    }
}
