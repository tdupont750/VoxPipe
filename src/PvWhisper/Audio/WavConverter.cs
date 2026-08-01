using System.Runtime.InteropServices;
using System.Text;

namespace PvWhisper.Audio;

public sealed class WavConverter
{
    // RIFF chunk (12) + fmt subchunk (24) + data subchunk header (8)
    private const int HeaderBytes = 44;

    public MemoryStream CreateWavFromPcm16(short[] samples, int sampleRate)
    {
        int bytesPerSample = 2;
        short numChannels = 1;
        short bitsPerSample = 16;
        int byteRate = sampleRate * numChannels * bytesPerSample;
        short blockAlign = (short)(numChannels * bytesPerSample);
        int subchunk2Size = samples.Length * bytesPerSample;
        int chunkSize = 4 + (8 + 16) + (8 + subchunk2Size);

        // Exact capacity, so the stream never has to grow and copy itself
        var ms = new MemoryStream(HeaderBytes + subchunk2Size);

        using (var writer = new BinaryWriter(ms, Encoding.UTF8, leaveOpen: true))
        {
            writer.Write(Encoding.ASCII.GetBytes("RIFF"));
            writer.Write(chunkSize);
            writer.Write(Encoding.ASCII.GetBytes("WAVE"));

            writer.Write(Encoding.ASCII.GetBytes("fmt "));
            writer.Write(16);
            writer.Write((short)1);
            writer.Write(numChannels);
            writer.Write(sampleRate);
            writer.Write(byteRate);
            writer.Write(blockAlign);
            writer.Write(bitsPerSample);

            writer.Write(Encoding.ASCII.GetBytes("data"));
            writer.Write(subchunk2Size);

            // BinaryWriter.Write(short) is little-endian by contract while MemoryMarshal
            // reinterprets in native order, so only take the bulk path when they agree.
            if (BitConverter.IsLittleEndian)
            {
                writer.Write(MemoryMarshal.AsBytes(samples.AsSpan()));
            }
            else
            {
                foreach (var s in samples)
                    writer.Write(s);
            }
        }

        ms.Position = 0;
        return ms;
    }
}
