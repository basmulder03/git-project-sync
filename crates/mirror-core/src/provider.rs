use crate::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo};
use anyhow::bail;
use std::future::Future;
use std::pin::Pin;
use std::task::{Context, Poll, RawWaker, RawWakerVTable, Waker};

pub type ProviderFuture<'a, T> = Pin<Box<dyn Future<Output = anyhow::Result<T>> + 'a>>;

fn noop_raw_waker() -> RawWaker {
    unsafe fn clone(_: *const ()) -> RawWaker {
        noop_raw_waker()
    }
    unsafe fn wake(_: *const ()) {}
    unsafe fn wake_by_ref(_: *const ()) {}
    unsafe fn drop(_: *const ()) {}
    RawWaker::new(
        std::ptr::null(),
        &RawWakerVTable::new(clone, wake, wake_by_ref, drop),
    )
}

pub fn block_on<F: Future>(future: F) -> F::Output {
    let waker = unsafe { Waker::from_raw(noop_raw_waker()) };
    let mut future = std::pin::pin!(future);
    let mut cx = Context::from_waker(&waker);
    loop {
        match Future::poll(future.as_mut(), &mut cx) {
            Poll::Ready(value) => return value,
            Poll::Pending => std::thread::yield_now(),
        }
    }
}

pub trait RepoProvider {
    fn kind(&self) -> ProviderKind;
    fn list_repos<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, Vec<RemoteRepo>>;
    fn validate_auth<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, ()>;
    fn health_check<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, ()> {
        Box::pin(async move { self.validate_auth(target).await })
    }
    fn auth_for_target<'a>(
        &'a self,
        _target: &'a ProviderTarget,
    ) -> ProviderFuture<'a, Option<crate::model::RepoAuth>> {
        Box::pin(async { Ok(None) })
    }
    fn token_scopes<'a>(
        &'a self,
        _target: &'a ProviderTarget,
    ) -> ProviderFuture<'a, Option<Vec<String>>> {
        Box::pin(async { Ok(None) })
    }
    fn register_webhook<'a>(
        &'a self,
        _target: &'a ProviderTarget,
        _url: &str,
        _secret: Option<&str>,
    ) -> ProviderFuture<'a, ()> {
        Box::pin(async { bail!("webhook registration not supported for this provider") })
    }

    fn get_repo<'a>(
        &'a self,
        _target: &'a ProviderTarget,
        _repo_id: &str,
    ) -> ProviderFuture<'a, Option<RemoteRepo>> {
        Box::pin(async { Ok(None) })
    }
}

pub trait ProviderSpec {
    fn kind(&self) -> ProviderKind;
    fn default_host(&self) -> &'static str;
    fn parse_scope(&self, segments: Vec<String>) -> anyhow::Result<ProviderScope>;
    fn validate_scope(&self, scope: &ProviderScope) -> anyhow::Result<()>;
    fn account_key(&self, host: &str, scope: &ProviderScope) -> anyhow::Result<String>;
}
