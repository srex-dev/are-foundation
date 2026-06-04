#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum GlobError {
    #[error("invalid pattern")]
    InvalidPattern,
    /// Pathological `**` nesting, `*` backtracking, or total work exceeded (deny match; avoids ReDoS / stack overflow).
    #[error("pattern too complex")]
    PatternTooComplex,
}

/// Maximum consecutive `**` segments processed while matching (caps recursion / branch factor).
const MAX_DOUBLE_STAR_DEPTH: u8 = 48;

/// Cap for segment matching: each tick is one matcher step (recursive entry, `*` trial, or `**` branch).
const MAX_GLOB_WORK: u32 = 65_536;

fn burn_work(work: &mut u32, cost: u32) -> Result<(), GlobError> {
    let w = work.checked_sub(cost).ok_or(GlobError::PatternTooComplex)?;
    *work = w;
    Ok(())
}

pub fn glob_match(pattern: &str, resource: &str) -> Result<bool, GlobError> {
    if pattern.is_empty() {
        return Ok(false);
    }
    let mut work = MAX_GLOB_WORK;
    let pattern_segments: Vec<&str> = pattern.split('/').collect();
    let resource_segments: Vec<&str> = if resource.is_empty() {
        vec![]
    } else {
        resource.split('/').collect()
    };
    match_segments(&pattern_segments, &resource_segments, 0, &mut work)
}

fn match_segments(
    pattern: &[&str],
    resource: &[&str],
    double_star_depth: u8,
    work: &mut u32,
) -> Result<bool, GlobError> {
    burn_work(work, 1)?;
    if pattern.is_empty() {
        return Ok(resource.is_empty());
    }
    if pattern[0] == "**" {
        if double_star_depth >= MAX_DOUBLE_STAR_DEPTH {
            return Err(GlobError::PatternTooComplex);
        }
        if pattern.len() == 1 {
            return Ok(true);
        }
        for i in 0..=resource.len() {
            burn_work(work, 1)?;
            if match_segments(&pattern[1..], &resource[i..], double_star_depth + 1, work)? {
                return Ok(true);
            }
        }
        return Ok(false);
    }
    if resource.is_empty() {
        return Ok(false);
    }
    if !match_segment(pattern[0], resource[0], work)? {
        return Ok(false);
    }
    match_segments(&pattern[1..], &resource[1..], double_star_depth, work)
}

fn match_segment(pattern: &str, text: &str, work: &mut u32) -> Result<bool, GlobError> {
    burn_work(work, 1)?;
    let mut p = pattern.chars().peekable();
    let t: Vec<char> = text.chars().collect();
    let mut ti = 0usize;

    while let Some(ch) = p.next() {
        match ch {
            '*' => {
                while ti < t.len() {
                    burn_work(work, 1)?;
                    let suffix: String = t[ti..].iter().collect();
                    let rest: String = p.clone().collect();
                    if match_segment(&rest, &suffix, work)? {
                        return Ok(true);
                    }
                    ti += 1;
                }
                return Ok(p.peek().is_none());
            }
            '?' => {
                if ti >= t.len() {
                    return Ok(false);
                }
                ti += 1;
            }
            '[' | ']' | '{' | '}' => return Err(GlobError::InvalidPattern),
            literal => {
                if ti >= t.len() || t[ti] != literal {
                    return Ok(false);
                }
                ti += 1;
            }
        }
    }

    Ok(ti == t.len())
}

#[cfg(test)]
mod tests {
    use super::glob_match;

    #[test]
    fn comprehensive_glob_cases() {
        let cases = vec![
            ("data/*", "data/users", true),
            ("data/*", "data/users/123", false),
            ("data/**", "data/users/123/profile", true),
            ("data/**", "other/users", false),
            ("*", "anything", true),
            ("**", "any/thing/at/all", true),
            ("data/user-?", "data/user-a", true),
            ("data/user-?", "data/user-ab", false),
            ("exact/resource", "exact/resource", true),
            ("exact/resource", "exact/Resource", false),
            ("", "anything", false),
            ("data/*", "", false),
            ("logs/**", "logs", true),
            ("logs/*", "logs", false),
            ("a/*/c", "a/b/c", true),
            ("a/*/c", "a/b/d", false),
            ("a/**/c", "a/b/d/c", true),
            ("a/**/c", "a/c", true),
            ("a/**/c", "a/b", false),
            ("foo-??", "foo-ab", true),
            ("foo-??", "foo-a", false),
            ("**/admin", "a/b/admin", true),
            ("**/admin", "a/b/user", false),
            ("root/**/leaf", "root/x/y/leaf", true),
            ("root/**/leaf", "root/leaf", true),
            ("root/*/leaf", "root/x/leaf", true),
            ("root/*/leaf", "root/x/y/leaf", false),
            ("abc*", "abcdef", true),
            ("abc*", "abc", true),
            ("abc*", "ab", false),
            ("tenant/*/project/*", "tenant/a/project/b", true),
            ("tenant/*/project/*", "tenant/a/project/b/c", false),
            ("**/v1/*", "api/svc/v1/list", true),
            ("**/v1/*", "api/svc/v2/list", false),
            ("a?c", "abc", true),
            ("a?c", "ac", false),
            ("read/**", "read", true),
            ("read/**", "read/x", true),
            ("read/*", "read/x", true),
            ("read/*", "read/x/y", false),
            ("x/**/z", "x/y/z", true),
            ("x/**/z", "x/y/a/z", true),
            ("x/**/z", "x/y/a", false),
            ("**", "", true),
            ("*/a", "x/a", true),
            ("*/a", "x/y/a", false),
            ("prefix-*", "prefix-123", true),
            ("prefix-*", "pre-123", false),
            ("**/metrics", "metrics", true),
            ("**/metrics", "a/b/metrics", true),
            ("**/metrics", "a/b/metric", false),
        ];

        for (pattern, resource, expected) in cases {
            let got = glob_match(pattern, resource).expect("pattern should be valid");
            assert_eq!(got, expected, "pattern={pattern} resource={resource}");
        }
    }

    #[test]
    fn invalid_pattern_is_reported() {
        assert!(glob_match("data/[abc]", "data/a").is_err());
    }

    #[test]
    fn excessive_double_star_nesting_fails_closed() {
        let mut pattern = String::new();
        for _ in 0..60 {
            pattern.push_str("**/");
        }
        pattern.push('x');
        assert_eq!(
            glob_match(&pattern, "a/b/c/x"),
            Err(super::GlobError::PatternTooComplex)
        );
    }

    /// Many consecutive `*` in one segment against long text exhausts the work budget (backtracking).
    #[test]
    fn star_backtracking_budget_fails_closed() {
        let mut pattern = String::new();
        for _ in 0..35 {
            pattern.push('*');
        }
        pattern.push('z');
        // No `z` in resource: matcher explores many `*` splits and must not run unbounded.
        let resource = "a".repeat(8000);
        assert_eq!(
            glob_match(&pattern, &resource),
            Err(super::GlobError::PatternTooComplex)
        );
    }

    #[test]
    fn normal_star_still_matches() {
        assert!(glob_match("foo*bar", "foooooobar").unwrap());
    }
}
