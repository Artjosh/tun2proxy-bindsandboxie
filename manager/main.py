import tkinter as tk
from tkinter import filedialog, messagebox
import customtkinter as ctk
import os
import sys
import ctypes
import threading

from config_manager import ConfigManager
from proxy_engine import ProxyEngine
from sandbox_manager import SandboxManager

ctk.set_appearance_mode("Dark")
ctk.set_default_color_theme("blue")

class ProxyManagerApp(ctk.CTk):
    def __init__(self):
        super().__init__()

        self.title("Arcanum Proxy Manager")
        self.geometry("1000x800")

        self.config = ConfigManager()
        self.proxy_engine = ProxyEngine(self.config.get_path("tun2socks"))
        self.sandbox_manager = SandboxManager(self.config)
        
        self.grid_columnconfigure(0, weight=1)
        self.grid_rowconfigure(1, weight=1) # Tabview is row 1

        self._create_header()
        self._create_tabs()
        
        # Load initial state
        self._load_saved_data()

    def _create_header(self):
        header_frame = ctk.CTkFrame(self)
        header_frame.grid(row=0, column=0, padx=20, pady=10, sticky="ew")
        
        title_label = ctk.CTkLabel(header_frame, text="Arcanum Controller", font=("Roboto", 24, "bold"))
        title_label.pack(side="left", padx=20, pady=10)

        # Admin status
        is_admin = ctypes.windll.shell32.IsUserAnAdmin() != 0
        admin_color = "green" if is_admin else "red"
        admin_text = "ADMIN: YES" if is_admin else "ADMIN: NO (Re-run as Admin)"
        
        admin_label = ctk.CTkLabel(header_frame, text=admin_text, text_color=admin_color)
        admin_label.pack(side="right", padx=20)

    def _create_tabs(self):
        self.tabview = ctk.CTkTabview(self)
        self.tabview.grid(row=1, column=0, padx=20, pady=10, sticky="nsew")
        
        self.tab_dashboard = self.tabview.add("Dashboard")
        self.tab_config = self.tabview.add("Configuration")
        
        self._build_dashboard_tab()
        self._build_config_tab()

    def _build_config_tab(self):
        frame = self.tab_config
        
        # Paths
        ctk.CTkLabel(frame, text="Application Paths", font=("Roboto", 16, "bold")).pack(pady=10)
        
        self.entry_tun2socks = self._create_path_selector(frame, "tun2socks.exe Path:", "tun2socks")
        self.entry_wintun = self._create_path_selector(frame, "wintun.dll Path:", "wintun")
        self.entry_sbie_ini = self._create_path_selector(frame, "Sandboxie.ini Path:", "sandboxie_ini")
        self.entry_sbie_exe = self._create_path_selector(frame, "SbieIni.exe Path:", "sbie_ini_exe")

    def _create_path_selector(self, parent, label_text, config_key):
        container = ctk.CTkFrame(parent)
        container.pack(fill="x", padx=20, pady=5)
        
        ctk.CTkLabel(container, text=label_text, width=150).pack(side="left", padx=10)
        
        entry = ctk.CTkEntry(container)
        entry.pack(side="left", fill="x", expand=True, padx=10)
        entry.insert(0, self.config.get_path(config_key))
        
        def pick_file():
            val = filedialog.askopenfilename()
            if val:
                entry.delete(0, "end")
                entry.insert(0, val)
                self.config.set_path(config_key, val)
                # Update engine path if it's tun2socks
                if config_key == "tun2socks":
                    self.proxy_engine.tun2socks_path = val

        ctk.CTkButton(container, text="Browse", width=80, command=pick_file).pack(side="right", padx=10)
        return entry

    def _build_dashboard_tab(self):
        frame = self.tab_dashboard
        frame.grid_columnconfigure(0, weight=1)
        frame.grid_rowconfigure(2, weight=1) # Sandbox content area

        # 1. Proxy Control
        proxy_frame = ctk.CTkFrame(frame)
        proxy_frame.grid(row=0, column=0, padx=10, pady=10, sticky="ew")
        
        ctk.CTkLabel(proxy_frame, text="Proxy Management", font=("Roboto", 16, "bold")).pack(pady=5)
        
        self.txt_proxies = ctk.CTkTextbox(proxy_frame, height=100)
        self.txt_proxies.pack(fill="x", padx=20, pady=5)
        self.txt_proxies.insert("0.0", "IP:PORT:USER:PASS") # Placeholder

        btn_row = ctk.CTkFrame(proxy_frame, fg_color="transparent")
        btn_row.pack(pady=10)
        
        self.btn_start = ctk.CTkButton(btn_row, text="START PROXIES (tun2socks)", fg_color="green", command=self.start_proxies, width=200)
        self.btn_start.pack(side="left", padx=10)
        
        self.btn_stop = ctk.CTkButton(btn_row, text="STOP ALL", fg_color="firebrick", command=self.stop_proxies, width=200)
        self.btn_stop.pack(side="left", padx=10)
        
        # 2. Sandbox Shortcuts
        sb_control_frame = ctk.CTkFrame(frame)
        sb_control_frame.grid(row=1, column=0, padx=10, pady=5, sticky="ew")
        
        self.lbl_shortcuts_dir = ctk.CTkLabel(sb_control_frame, text="Atalhos: None")
        self.lbl_shortcuts_dir.pack(side="left", padx=10)
        
        ctk.CTkButton(sb_control_frame, text="Select Shortcut Folder", command=self.select_shortcut_folder).pack(side="right", padx=10)
        
        # 3. Dynamic Content
        self.scrollable = ctk.CTkScrollableFrame(frame, label_text="Sandboxes")
        self.scrollable.grid(row=2, column=0, padx=10, pady=10, sticky="nsew")

    def _load_saved_data(self):
        # Load proxies
        saved_proxies = self.config.get("proxies_list", [])
        if saved_proxies:
            self.txt_proxies.delete("0.0", "end")
            text = "\n".join(saved_proxies)
            self.txt_proxies.insert("0.0", text)
        
        # Load folder
        folder = self.config.get("last_shortcuts_dir", "")
        if folder and os.path.exists(folder):
            self.lbl_shortcuts_dir.configure(text=f"Atalhos: {folder}")
            self.refresh_shortcuts(folder)

    def start_proxies(self):
        raw_text = self.txt_proxies.get("0.0", "end")
        proxies = self.proxy_engine.parse_proxies(raw_text)
        if not proxies:
            messagebox.showwarning("Warning", "No valid proxies found!")
            return
            
        # Save to config
        lines = [x.strip() for x in raw_text.split('\n') if x.strip()]
        self.config.set("proxies_list", lines)
        
        self.btn_start.configure(state="disabled", text="Starting...")
        
        def run():
            self.proxy_engine.start_proxies(proxies, log_callback=print)
            self.btn_start.configure(state="normal", text="START PROXIES (tun2socks)")
            # Refresh adapters in UI after some time
            self.after(5000, self.refresh_adapters_dropdowns)

        threading.Thread(target=run, daemon=True).start()

    def stop_proxies(self):
        self.proxy_engine.stop_all()
        messagebox.showinfo("Info", "Stopped existing tun2socks processes.")

    def select_shortcut_folder(self):
        path = filedialog.askdirectory()
        if path:
            self.config.set("last_shortcuts_dir", path)
            self.lbl_shortcuts_dir.configure(text=f"Atalhos: {path}")
            self.refresh_shortcuts(path)

    def refresh_shortcuts(self, path):
        # Clear existing
        for widget in self.scrollable.winfo_children():
            widget.destroy()
            
        groups = self.sandbox_manager.scan_shortcuts(path)
        adapters = self.sandbox_manager.get_available_adapters()
        self.adapter_options = adapters # Cache for updates
        
        sorted_ids = sorted(groups.keys(), key=lambda x: int(x))
        
        for gid in sorted_ids:
            shortcuts = groups[gid]
            if not shortcuts: continue
            
            box_name = shortcuts[0].box_name
            
            # Row Frame
            row = ctk.CTkFrame(self.scrollable)
            row.pack(fill="x", pady=5, padx=5)
            
            # ID Label
            ctk.CTkLabel(row, text=f"#{gid} [{box_name}]", font=("Roboto", 14, "bold"), width=120).pack(side="left", padx=10)
            
            # Application Buttons
            apps_frame = ctk.CTkFrame(row, fg_color="transparent")
            apps_frame.pack(side="left", fill="x", expand=True)
            
            for s in shortcuts:
                # Shorten name for button
                btn_text = s.app_name.replace(".exe", "").replace(".lnk", "").capitalize()
                ctk.CTkButton(apps_frame, text=btn_text, width=100, 
                              command=lambda p=s.path: self.sandbox_manager.launch_shortcut(p)).pack(side="left", padx=5)

            # Bind Adapter Controls
            bind_frame = ctk.CTkFrame(row, fg_color="transparent")
            bind_frame.pack(side="right", padx=10)
            
            # Current Bind logic
            current_bind = self.sandbox_manager.get_bind_adapter_for_box(box_name)
            
            # Dropdown
            var = ctk.StringVar(value=current_bind)
            dropdown = ctk.CTkOptionMenu(bind_frame, values=adapters, variable=var, width=150)
            dropdown.pack(side="left", padx=5)
            
            # Bind Button
            def apply_bind(bname=box_name, v=var):
                sel = v.get()
                self.sandbox_manager.set_bind_adapter(bname, sel)
                print(f"Set {bname} to {sel}")
                
            ctk.CTkButton(bind_frame, text="Bind", width=60, fg_color="#3B8ED0", command=apply_bind).pack(side="left")

    def refresh_adapters_dropdowns(self):
        # This is a bit complex dynamically, but for now user can just re-select folder to refresh
        # Or we implement a global refresh.
        # For MVP, re-selecting folder triggers refresh.
        pass

if __name__ == "__main__":
    if not ctypes.windll.shell32.IsUserAnAdmin():
        # Re-run as admin
        ctypes.windll.shell32.ShellExecuteW(None, "runas", sys.executable, " ".join(sys.argv), None, 1)
    else:
        app = ProxyManagerApp()
        app.mainloop()
