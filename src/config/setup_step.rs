//! `SetupStep` 加载时的 wire-format 反序列化(单键契约)。
//!
//! 严格对应 Go 版 `setup_yaml.go`:只接受 `copy:` / `run:` 中的**一个**,
//! 其他 key 或零键 / 多键均报错。

use serde::de::{self, Deserialize, Deserializer, MapAccess, Visitor};
use std::fmt;

use super::{CopyAction, RunAction, SetupStep};

/// 中间结构,与 Go 端 `UnmarshalYAML(value *yaml.Node)` 一一对应。
/// 此结构不暴露给 `Config.setup`;后者在 `load` 时被映射为 [`SetupStep`].
#[derive(Debug)]
pub struct SetupStepWire(pub SetupStep);

impl<'de> Deserialize<'de> for SetupStepWire {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        struct StepVisitor;

        impl<'de> Visitor<'de> for StepVisitor {
            type Value = SetupStepWire;

            fn expecting(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
                f.write_str("a setup step mapping with exactly one of: copy, run")
            }

            fn visit_map<M>(self, mut map: M) -> Result<Self::Value, M::Error>
            where
                M: MapAccess<'de>,
            {
                let mut copy: Option<CopyAction> = None;
                let mut run: Option<RunAction> = None;
                let mut other: Option<String> = None;

                // First key: remember its name; reject any subsequent key.
                let mut first_key: Option<String> = None;
                let mut extra_key: Option<String> = None;

                while let Some(key) = map.next_key::<String>()? {
                    if first_key.is_none() {
                        first_key = Some(key.clone());
                    } else {
                        extra_key = Some(key);
                        // Still need to consume value to keep map iteration valid.
                        let _: serde::de::IgnoredAny = map.next_value()?;
                        continue;
                    }
                    match key.as_str() {
                        "copy" => {
                            if copy.is_some() || run.is_some() || other.is_some() {
                                return Err(de::Error::custom("step must have exactly one action key"));
                            }
                            let v: CopyAction = map.next_value()?;
                            copy = Some(v);
                        }
                        "run" => {
                            if copy.is_some() || run.is_some() || other.is_some() {
                                return Err(de::Error::custom("step must have exactly one action key"));
                            }
                            let v: RunAction = map.next_value()?;
                            run = Some(v);
                        }
                        _ => {
                            if copy.is_some() || run.is_some() {
                                return Err(de::Error::custom("step must have exactly one action key"));
                            }
                            other = Some(key);
                            let _: serde::de::IgnoredAny = map.next_value()?;
                        }
                    }
                }

                if extra_key.is_some() {
                    return Err(de::Error::custom("step must have exactly one action key"));
                }

                match (copy, run, other) {
                    (Some(c), None, None) => Ok(SetupStepWire(SetupStep::copy(c))),
                    (None, Some(r), None) => Ok(SetupStepWire(SetupStep::run(r))),
                    (None, None, Some(name)) => {
                        Err(de::Error::custom(format!("unknown setup action {name:?}")))
                    }
                    (None, None, None) => {
                        Err(de::Error::custom("setup step must be a mapping with one action key"))
                    }
                    _ => unreachable!("guarded by extra_key and per-key checks above"),
                }
            }
        }

        deserializer.deserialize_map(StepVisitor)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_saphyr as yaml;

    fn parse_one(s: &str) -> Result<SetupStep, String> {
        let v: SetupStepWire = yaml::from_str(s).map_err(|e| e.to_string())?;
        Ok(v.0)
    }

    #[test]
    fn parses_copy_step() {
        let step = parse_one("copy: { from: a, to: b }").unwrap();
        assert!(step.copy.is_some());
        assert!(step.run.is_none());
    }

    #[test]
    fn parses_run_step() {
        let step = parse_one("run: { command: \"echo hi\" }").unwrap();
        assert!(step.run.is_some());
        assert!(step.copy.is_none());
    }

    #[test]
    fn rejects_two_keys() {
        let err = parse_one("copy: { from: a, to: b }\nrun: { command: x }").unwrap_err();
        assert!(err.contains("exactly one"), "got: {err}");
    }

    #[test]
    fn rejects_unknown_action() {
        let err = parse_one("mkdir: foo").unwrap_err();
        assert!(err.contains("unknown setup action"), "got: {err}");
    }

    #[test]
    fn rejects_empty_mapping() {
        let err = parse_one("{}").unwrap_err();
        // serde-saphyr 不接受空 map / 非 mapping;出现 "expected mapping" 之类
        assert!(!err.is_empty(), "should error");
    }
}
