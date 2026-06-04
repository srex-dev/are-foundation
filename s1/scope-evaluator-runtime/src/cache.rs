use std::collections::{HashMap, VecDeque};
use std::time::{Duration, Instant};

use crate::types::Passport;

#[derive(Clone)]
pub struct CachedPassport {
    pub passport: Passport,
    pub cached_at: Instant,
}

pub struct PassportCache {
    entries: HashMap<String, CachedPassport>,
    lru: VecDeque<String>,
    max_entries: usize,
    ttl: Duration,
}

impl PassportCache {
    pub fn new(max_entries: usize, ttl_seconds: u64) -> Self {
        Self {
            entries: HashMap::new(),
            lru: VecDeque::new(),
            max_entries,
            ttl: Duration::from_secs(ttl_seconds),
        }
    }

    pub fn get(&mut self, agent_id: &str) -> Option<Passport> {
        let entry = self.entries.get(agent_id)?.clone();
        if entry.cached_at.elapsed() > self.ttl {
            self.remove(agent_id);
            return None;
        }
        self.touch(agent_id);
        Some(entry.passport)
    }

    pub fn insert(&mut self, agent_id: String, passport: Passport) {
        if self.entries.contains_key(&agent_id) {
            self.remove(&agent_id);
        }
        self.entries.insert(
            agent_id.clone(),
            CachedPassport {
                passport,
                cached_at: Instant::now(),
            },
        );
        self.lru.push_back(agent_id);
        while self.entries.len() > self.max_entries {
            if let Some(oldest) = self.lru.pop_front() {
                self.entries.remove(&oldest);
            } else {
                break;
            }
        }
    }

    pub fn invalidate(&mut self, agent_id: &str) -> bool {
        self.remove(agent_id)
    }

    pub fn len(&self) -> usize {
        self.entries.len()
    }

    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    fn remove(&mut self, agent_id: &str) -> bool {
        let removed = self.entries.remove(agent_id).is_some();
        if removed {
            self.lru.retain(|k| k != agent_id);
        }
        removed
    }

    fn touch(&mut self, agent_id: &str) {
        self.lru.retain(|k| k != agent_id);
        self.lru.push_back(agent_id.to_string());
    }
}

#[cfg(test)]
mod tests {
    use super::PassportCache;
    use crate::types::Passport;

    #[test]
    fn lru_eviction_and_invalidate() {
        let mut cache = PassportCache::new(2, 60);
        cache.insert(
            "a".to_string(),
            Passport {
                passport_id: "p1".into(),
                passport_type: "STANDARD".into(),
                expires_ts: i64::MAX,
                scopes: vec![],
            },
        );
        cache.insert(
            "b".to_string(),
            Passport {
                passport_id: "p2".into(),
                passport_type: "STANDARD".into(),
                expires_ts: i64::MAX,
                scopes: vec![],
            },
        );
        assert!(cache.get("a").is_some());
        cache.insert(
            "c".to_string(),
            Passport {
                passport_id: "p3".into(),
                passport_type: "STANDARD".into(),
                expires_ts: i64::MAX,
                scopes: vec![],
            },
        );
        assert!(cache.get("b").is_none());
        assert!(cache.invalidate("a"));
        assert!(!cache.invalidate("a"));
    }

    #[test]
    fn ttl_expiration_removes_entry() {
        let mut cache = PassportCache::new(1, 0);
        cache.insert(
            "a".to_string(),
            Passport {
                passport_id: "p1".into(),
                passport_type: "STANDARD".into(),
                expires_ts: i64::MAX,
                scopes: vec![],
            },
        );
        assert!(cache.get("a").is_none());
    }

    #[test]
    fn update_existing_and_size_helpers() {
        let mut cache = PassportCache::new(2, 60);
        assert!(cache.is_empty());
        cache.insert(
            "a".to_string(),
            Passport {
                passport_id: "p1".into(),
                passport_type: "STANDARD".into(),
                expires_ts: i64::MAX,
                scopes: vec![],
            },
        );
        cache.insert(
            "a".to_string(),
            Passport {
                passport_id: "p2".into(),
                passport_type: "STANDARD".into(),
                expires_ts: i64::MAX,
                scopes: vec![],
            },
        );
        assert_eq!(cache.len(), 1);
        let passport = cache.get("a").expect("entry should exist");
        assert_eq!(passport.passport_id, "p2");
    }
}
