// env::var("FAKE_IN_COMMENT")
use std::env;

fn read<'a>(fallback: &'a str) -> String {
    let url = env::var("REAL_DATABASE_URL").expect("set it");
    let port = std::env::var("REAL_PORT").unwrap_or_else(|_| fallback.to_string());
    let version = env!("REAL_BUILD_VERSION");
    let sha = option_env!("REAL_GIT_SHA");
    let text = r#"env::var("FAKE_IN_RAW_STRING")"#;
    let quote = '"';
    format!("{url}{port}{version}{sha:?}{text}{quote}")
}
