//! Parser for `git worktree list --porcelain` output.
//!
//! 与 Go 版 `parseWorktreePorcelain(out string) []Worktree` 1:1 等价。

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Worktree {
    pub path: String,
    pub head: String,
    /// Short branch name (`refs/heads/` stripped). Empty when detached / bare.
    pub branch: String,
    pub bare: bool,
    pub detached: bool,
}

pub fn parse_worktree_porcelain(out: &str) -> Vec<Worktree> {
    let mut entries: Vec<Worktree> = Vec::new();
    let mut cur: Option<Worktree> = None;
    let mut have = false;

    let flush = |cur: &mut Option<Worktree>, have: &mut bool, entries: &mut Vec<Worktree>| {
        if let Some(w) = cur.take()
            && *have
        {
            entries.push(w);
        }
        *have = false;
    };

    for raw in out.split('\n') {
        let line = raw.trim_end_matches('\r');
        if line.is_empty() {
            flush(&mut cur, &mut have, &mut entries);
            continue;
        }
        let (key, val) = match line.split_once(' ') {
            Some((k, v)) => (k, v),
            None => (line, ""),
        };
        match key {
            "worktree" => {
                flush(&mut cur, &mut have, &mut entries);
                cur = Some(Worktree {
                    path: val.to_string(),
                    head: String::new(),
                    branch: String::new(),
                    bare: false,
                    detached: false,
                });
                have = true;
            }
            "HEAD" => {
                if let Some(w) = cur.as_mut() {
                    w.head = val.to_string();
                    have = true;
                }
            }
            "branch" => {
                if let Some(w) = cur.as_mut() {
                    w.branch = val.strip_prefix("refs/heads/").unwrap_or(val).to_string();
                    have = true;
                }
            }
            "detached" => {
                if let Some(w) = cur.as_mut() {
                    w.detached = true;
                    have = true;
                }
            }
            "bare" => {
                if let Some(w) = cur.as_mut() {
                    w.bare = true;
                    have = true;
                }
            }
            _ => {} // ignore unknown keys (forward-compat)
        }
    }
    flush(&mut cur, &mut have, &mut entries);
    entries
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_main_only() {
        let input = "worktree /home/user/proj\nHEAD abc123\nbranch refs/heads/main\n\n";
        let wts = parse_worktree_porcelain(input);
        assert_eq!(wts.len(), 1);
        assert_eq!(wts[0].path, "/home/user/proj");
        assert_eq!(wts[0].branch, "main");
        assert!(!wts[0].bare);
        assert!(!wts[0].detached);
    }

    #[test]
    fn parses_with_worktree() {
        let input = "\
worktree /home/user/proj
HEAD abc123
branch refs/heads/main

worktree /home/user/proj/.worktrees/api/feat-x
HEAD def456
branch refs/heads/feat-x

";
        let wts = parse_worktree_porcelain(input);
        assert_eq!(wts.len(), 2);
        assert_eq!(wts[1].path, "/home/user/proj/.worktrees/api/feat-x");
        assert_eq!(wts[1].branch, "feat-x");
    }

    #[test]
    fn strips_refs_heads() {
        let input = "worktree /p\nHEAD h\nbranch refs/heads/foo/bar\n\n";
        let wts = parse_worktree_porcelain(input);
        assert_eq!(wts[0].branch, "foo/bar");
    }

    #[test]
    fn recognizes_detached() {
        let input = "worktree /p\nHEAD h\ndetached\n\n";
        let wts = parse_worktree_porcelain(input);
        assert!(wts[0].detached);
        assert_eq!(wts[0].branch, "");
    }

    #[test]
    fn recognizes_bare() {
        let input = "worktree /p.git\nHEAD h\nbare\n\n";
        let wts = parse_worktree_porcelain(input);
        assert!(wts[0].bare);
    }

    #[test]
    fn handles_crlf() {
        let input = "worktree /p\r\nHEAD h\r\nbranch refs/heads/main\r\n\r\n";
        let wts = parse_worktree_porcelain(input);
        assert_eq!(wts.len(), 1);
        assert_eq!(wts[0].branch, "main");
    }

    #[test]
    fn empty_input_yields_empty() {
        assert!(parse_worktree_porcelain("").is_empty());
    }

    #[test]
    fn trailing_newline_required_for_flush() {
        // Without trailing newline, last record still gets flushed.
        let input = "worktree /p\nHEAD h\nbranch refs/heads/main";
        let wts = parse_worktree_porcelain(input);
        assert_eq!(wts.len(), 1);
    }
}
