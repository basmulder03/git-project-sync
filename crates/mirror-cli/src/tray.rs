use crate::tui;
use mirror_core::audit::{AuditContext, AuditLogger, AuditStatus};
use std::process::Command;
use std::sync::Arc;
use tray_icon::{
    TrayIconBuilder,
    menu::{Menu, MenuEvent, MenuItem},
};
use winit::event::{Event, StartCause};
use winit::event_loop::{ControlFlow, EventLoop};

pub fn run_tray(audit: &AuditLogger) -> anyhow::Result<()> {
    if is_headless() {
        eprintln!("System tray not available: no graphical session detected.");
        return Ok(());
    }
    let _ = audit.record("tray.start", AuditStatus::Ok, Some("tray"), None, None)?;
    let event_loop = EventLoop::new();
    let menu = Menu::new();
    let open_dashboard = MenuItem::new("Open Dashboard", true, None);
    let open_tui = MenuItem::new("Open TUI", true, None);
    let sync_now = MenuItem::new("Sync Now", true, None);
    let quit = MenuItem::new("Quit", true, None);

    menu.append(&open_dashboard)?;
    menu.append(&open_tui)?;
    menu.append(&sync_now)?;
    menu.append(&quit)?;

    let icon = match tray_icon::Icon::from_rgba(build_icon_rgba(), 16, 16) {
        Ok(icon) => icon,
        Err(_) => {
            eprintln!("System tray not available: failed to create tray icon.");
            return Ok(());
        }
    };

    let _tray_icon = match TrayIconBuilder::new()
        .with_menu(Box::new(menu))
        .with_tooltip("Git Project Sync")
        .with_icon(icon)
        .build()
    {
        Ok(icon) => icon,
        Err(err) => {
            eprintln!("System tray not available: {err}");
            return Ok(());
        }
    };

    let audit = Arc::new(audit.clone());
    let current_exe = std::env::current_exe().ok();

    let menu_receiver = MenuEvent::receiver();
    let open_dashboard_id = open_dashboard.id();
    let open_tui_id = open_tui.id();
    let sync_now_id = sync_now.id();
    let quit_id = quit.id();

    event_loop.run(move |event, _, control_flow| {
        *control_flow = ControlFlow::Wait;
        match event {
            Event::NewEvents(StartCause::Init) => {}
            Event::UserEvent(_) => {}
            Event::WindowEvent { .. } => {}
            Event::MainEventsCleared => {}
            Event::RedrawEventsCleared => {}
            Event::LoopDestroyed => {}
            Event::AboutToWait => {
                while let Ok(event) = menu_receiver.try_recv() {
                    if event.id == open_dashboard_id {
                        let _ = audit.record_with_context(
                            "tray.open.dashboard",
                            AuditStatus::Ok,
                            Some("tray"),
                            AuditContext::empty(),
                            None,
                            None,
                        );
                        if let Some(exe) = current_exe.as_ref() {
                            let _ = Command::new(exe).arg("tui").arg("--dashboard").spawn();
                        } else {
                            let _ = tui::run_tui(audit.as_ref(), true);
                        }
                    } else if event.id == open_tui_id {
                        let _ = audit.record_with_context(
                            "tray.open.tui",
                            AuditStatus::Ok,
                            Some("tray"),
                            AuditContext::empty(),
                            None,
                            None,
                        );
                        if let Some(exe) = current_exe.as_ref() {
                            let _ = Command::new(exe).arg("tui").spawn();
                        } else {
                            let _ = tui::run_tui(audit.as_ref(), false);
                        }
                    } else if event.id == sync_now_id {
                        let _ = audit.record_with_context(
                            "tray.sync",
                            AuditStatus::Ok,
                            Some("tray"),
                            AuditContext::empty(),
                            None,
                            None,
                        );
                        if let Some(exe) = current_exe.as_ref() {
                            let _ = Command::new(exe)
                                .arg("sync")
                                .arg("--non-interactive")
                                .arg("--missing-remote")
                                .arg("skip")
                                .spawn();
                        }
                    } else if event.id == quit_id {
                        let _ = audit.record_with_context(
                            "tray.quit",
                            AuditStatus::Ok,
                            Some("tray"),
                            AuditContext::empty(),
                            None,
                            None,
                        );
                        *control_flow = ControlFlow::Exit;
                    }
                }
            }
            _ => {}
        }
    });
}

fn is_headless() -> bool {
    if cfg!(target_os = "linux") {
        let has_display = std::env::var("DISPLAY").is_ok();
        let has_wayland = std::env::var("WAYLAND_DISPLAY").is_ok();
        return !has_display && !has_wayland;
    }
    false
}

fn build_icon_rgba() -> Vec<u8> {
    let mut bytes = vec![0u8; 16 * 16 * 4];
    for y in 0..16 {
        for x in 0..16 {
            let idx = (y * 16 + x) * 4;
            let is_border = x == 0 || y == 0 || x == 15 || y == 15;
            let color = if is_border { (20, 20, 20) } else { (66, 135, 245) };
            bytes[idx] = color.0;
            bytes[idx + 1] = color.1;
            bytes[idx + 2] = color.2;
            bytes[idx + 3] = 255;
        }
    }
    bytes
}
