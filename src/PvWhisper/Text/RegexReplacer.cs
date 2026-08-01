using System.Collections.Concurrent;
using System.Globalization;
using System.Text.RegularExpressions;

namespace PvWhisper.Text;

public sealed partial class RegexReplacer
{
    // Patterns come from AppConfig.json and are fixed for the process lifetime, so the
    // cache stays small. The cap only guards against a caller supplying unbounded patterns.
    private const int MaxCachedPatterns = 64;

    private static readonly ConcurrentDictionary<(string Pattern, RegexOptions Options), Regex> PatternCache = new();

    // Tokens look like: {1}, {2:ToUpper}, {3:ToLower}
    // Group 1 = index, Group 2 = operation
    [GeneratedRegex(@"\{(\d+)(?::(ToUpper|ToLower))?\}")]
    private static partial Regex TokenRegex();

    /// <summary>
    /// Replaces text in <paramref name="input"/> using a regex <paramref name="pattern"/> and a custom
    /// replacement template that supports tokens like {1}, {1:ToUpper}, {2:ToLower}.
    /// </summary>
    public string RegexReplace(
        string input,
        string pattern,
        string replaceTemplate,
        RegexOptions options = RegexOptions.None)
    {
        if (input == null) throw new ArgumentNullException(nameof(input));
        if (pattern == null) throw new ArgumentNullException(nameof(pattern));
        if (replaceTemplate == null) throw new ArgumentNullException(nameof(replaceTemplate));

        var regex = GetOrCreateRegex(pattern, options);

        return regex.Replace(input, match =>
        {
            // For each match, expand the template with this match's groups
            return ExpandTemplateForMatch(replaceTemplate, match);
        });
    }

    /// <summary>
    /// Constructing a Regex bypasses the runtime's own cache, so patterns are held here
    /// rather than being rebuilt on every call.
    /// </summary>
    private static Regex GetOrCreateRegex(string pattern, RegexOptions options)
    {
        var key = (pattern, options);

        if (PatternCache.TryGetValue(key, out var cached))
            return cached;

        // Past the cap, fall back to an uncached instance rather than growing without bound
        if (PatternCache.Count >= MaxCachedPatterns)
            return new Regex(pattern, options);

        return PatternCache.GetOrAdd(key, static k => new Regex(k.Pattern, k.Options));
    }

    private static string ExpandTemplateForMatch(string template, Match match)
    {
        string result = TokenRegex().Replace(template, m =>
        {
            int groupIndex = int.Parse(m.Groups[1].Value);
            if (groupIndex < 0 || groupIndex >= match.Groups.Count)
            {
                // Out-of-range group index – return empty or throw, depending on what you want
                return string.Empty;
            }

            string value = match.Groups[groupIndex].Value;

            if (!m.Groups[2].Success)
            {
                // No transform specified – just return the raw group value
                return value;
            }

            string op = m.Groups[2].Value;
            return op switch
            {
                "ToUpper" => value.ToUpper(CultureInfo.CurrentCulture),
                "ToLower" => value.ToLower(CultureInfo.CurrentCulture),
                _ => value
            };
        });

        return result;
    }
}
