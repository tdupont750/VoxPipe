using Pv;
using PvWhisper.Logging;

namespace PvWhisper.Audio;

public sealed class CaptureManager
{
    // PvRecorder always captures at 16 kHz
    private const int SampleRate = 16000;

    private PvRecorder? _recorder;
    private readonly DeviceResolver _deviceResolver;
    private readonly int _frameLength;
    private readonly int _bufferCapacity;
    private readonly Logger _logger;
    private readonly Lock _lock = new();

    private List<short>? _buffer;
    private Task? _captureTask;
    private CancellationTokenSource? _captureCts;

    public bool IsCapturing { get; private set; }

    public CaptureManager(DeviceResolver deviceResolver, int frameLength, int captureTimeoutSeconds, Logger logger)
    {
        _deviceResolver = deviceResolver;
        _frameLength = frameLength;
        _logger = logger;

        // Size the capture buffer up front so it never has to grow and copy mid-capture.
        // Clamped so that a very large (or absent) timeout still reserves something sane.
        var seconds = Math.Clamp(captureTimeoutSeconds > 0 ? captureTimeoutSeconds : 60, 1, 300);
        _bufferCapacity = SampleRate * seconds;
    }

    public Task StartCaptureAsync()
    {
        lock (_lock)
        {
            if (IsCapturing)
                return Task.CompletedTask;

            // Reused across captures; only the first one allocates
            _buffer ??= new List<short>(_bufferCapacity);
            _buffer.Clear();

            _captureCts = new CancellationTokenSource();
            IsCapturing = true;
            // Lazily create and start recorder only while actively capturing
            _recorder = PvRecorder.Create(frameLength: _frameLength, deviceIndex: _deviceResolver.Resolve());
            _recorder.Start();
            _captureTask = Task.Run(() => CaptureLoop(_captureCts.Token), _captureCts.Token);
        }

        return Task.CompletedTask;
    }

    public async Task StopCaptureAndDiscardAsync()
    {
        await ExtractAndStopAsync();
        RecycleBuffer();
    }

    public async Task<AudioBuffer?> StopCaptureAndGetSamplesAsync()
    {
        // If not currently capturing, return any remaining buffered samples
        lock (_lock)
        {
            if (!IsCapturing)
            {
                if (_buffer == null)
                    return null;

                var remaining = new AudioBuffer(_buffer.ToArray());
                _buffer.Clear();
                return remaining;
            }
        }

        var buffer = await ExtractAndStopAsync();
        if (buffer == null)
            return null;

        var audio = new AudioBuffer(buffer.ToArray());
        RecycleBuffer();
        return audio;
    }

    /// <summary>
    /// Returns the capture buffer to the reusable slot. A capture that ran far past the
    /// expected length is dropped instead, so one long session cannot pin its peak forever.
    /// </summary>
    private void RecycleBuffer()
    {
        lock (_lock)
        {
            if (_buffer == null)
                return;

            if (_buffer.Capacity > _bufferCapacity * 4)
                _buffer = null;
            else
                _buffer.Clear();
        }
    }

    /// <summary>
    /// Atomically marks capture as stopped, cancels the capture task, and disposes the recorder.
    /// Returns the buffer snapshot, or null if not capturing.
    /// </summary>
    private async Task<List<short>?> ExtractAndStopAsync()
    {
        List<short>? buffer;
        Task? captureTask;
        CancellationTokenSource? cts;

        lock (_lock)
        {
            if (!IsCapturing)
                return null;

            IsCapturing = false;
            // Kept for reuse; the caller copies out before RecycleBuffer clears it
            buffer = _buffer;
            cts = _captureCts; _captureCts = null;
            captureTask = _captureTask; _captureTask = null;
        }

        if (cts != null)
            await cts.CancelAsync();

        if (captureTask != null)
        {
            try { await captureTask; }
            catch (OperationCanceledException) { }
        }

        StopAndDisposeRecorderSafe();
        return buffer;
    }

    private void CaptureLoop(CancellationToken token)
    {
        try
        {
            while (!token.IsCancellationRequested)
            {
                var recorder = _recorder;
                if (recorder == null)
                    break;

                var frame = recorder.Read();

                lock (_lock)
                {
                    if (!IsCapturing || _buffer == null)
                        break;

                    _buffer.AddRange(frame);
                }
            }
        }
        catch (OperationCanceledException)
        {
            // normal
        }
        catch (Exception ex)
        {
            _logger.Error($"Capture loop error: {ex.Message}");
        }
        finally
        {
            StopAndDisposeRecorderSafe();
        }
    }

    private void StopAndDisposeRecorderSafe()
    {
        PvRecorder? recorderToDispose;
        lock (_lock)
        {
            recorderToDispose = _recorder;
            _recorder = null;
        }

        if (recorderToDispose == null)
            return;

        try { recorderToDispose.Stop(); } catch { /* ignore */ }
        try { recorderToDispose.Dispose(); } catch { /* ignore */ }
    }
}
