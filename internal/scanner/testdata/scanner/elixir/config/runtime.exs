# System.get_env("FAKE_IN_COMMENT")
import Config

config :app, App.Repo,
  url: System.fetch_env!("REAL_DATABASE_URL"),
  pool_size: String.to_integer(System.get_env("REAL_POOL_SIZE", "10"))
