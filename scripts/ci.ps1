$ErrorActionPreference = "Stop"

cargo fmt -- --check
cargo clippy --all-targets --all-features
cargo test --all
