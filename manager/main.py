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

        self.title("Joshboxie Tun2socks adapter manager and HWID changer")
        self.geometry("1000x800")

        self.config = ConfigManager()
        self.proxy_engine = ProxyEngine(self.config.get_path("tun2socks"))
        self.sandbox_manager = SandboxManager(self.config)
        
        self.grid_columnconfigure(0, weight=1)
        self.grid_rowconfigure(1, weight=1) # Tabview is row 1

        self._create_header()
        self._create_tabs()
        
        # Load initial state
        self.rows_cache = {}
        self._load_saved_data()

        # Start auto-refresh
        self.after(6000, self.auto_refresh_loop)

    def auto_refresh_loop(self):
        # Refresh shortcuts if a folder is selected
        fpath = self.config.get("last_shortcuts_dir", "")
        if fpath and os.path.exists(fpath):
             self.refresh_shortcuts(fpath)
        
        # Schedule next check in 6 seconds
        self.after(6000, self.auto_refresh_loop)

    def _create_header(self):
        header_frame = ctk.CTkFrame(self)
        header_frame.grid(row=0, column=0, padx=20, pady=10, sticky="ew")
        
        title_label = ctk.CTkLabel(header_frame, text="Joshboxie Tun2socks adapter manager and HWID changer", font=("Roboto", 18, "bold"))
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
            
            # 1. Clear UI and Cache
            self.rows_cache = {}
            for widget in self.scrollable.winfo_children():
                widget.destroy()
                
            # 2. Hide Scrollable and Show Loading *in its place*
            self.scrollable.grid_forget()
            
            self.loading_lbl = ctk.CTkLabel(self.tab_dashboard, text="Loading...", font=("Roboto", 20))
            self.loading_lbl.grid(row=2, column=0, pady=50)
            self.update() 
            
            # 3. Schedule the actual load
            self.after(50, lambda: self._finish_folder_load(path))

    def _finish_folder_load(self, path):
         # Perform the scan and populate (while invisible)
         self.refresh_shortcuts(path)

         # Remove loading and Restore Scrollable
         self.loading_lbl.destroy()
         self.scrollable.grid(row=2, column=0, padx=10, pady=10, sticky="nsew")

    def refresh_shortcuts(self, path):
        # Get Data
        groups = self.sandbox_manager.scan_shortcuts(path)
        adapters = self.sandbox_manager.get_available_adapters()
        self.adapter_options = adapters 
        
        # 1. Remove rows that no longer exist
        current_gids = set(groups.keys())
        cached_gids = set(self.rows_cache.keys())
        
        for gid in cached_gids - current_gids:
            self.rows_cache[gid]['frame'].destroy()
            del self.rows_cache[gid]
            
        sorted_ids = sorted(groups.keys(), key=lambda x: int(x))
        
        # 2. Update or Create rows
        for gid in sorted_ids:
            shortcuts = groups[gid]
            if not shortcuts: continue
            
            box_name = shortcuts[0].box_name
            current_bind = self.sandbox_manager.get_bind_adapter_for_box(box_name)
            is_spoofed = self.sandbox_manager.is_box_spoofed(box_name)
            shortcuts_sig = ",".join(s.name for s in shortcuts) 
            
            # Check if we have a cached row for this GID
            recreate = False
            if gid in self.rows_cache:
                if self.rows_cache[gid]['box_name'] != box_name:
                    self.rows_cache[gid]['frame'].destroy()
                    del self.rows_cache[gid]
                    recreate = True

            if gid in self.rows_cache and not recreate:
                # UPDATE EXISTING
                cache = self.rows_cache[gid]
                
                # --- Update Spoof Button ---
                current_spoof_text = "Spoofado" if is_spoofed else "Spoofar"
                current_spoof_color = "green" if is_spoofed else "#3B8ED0" # Blueish for action
                
                if cache['spoof_btn'].cget('text') != current_spoof_text:
                    cache['spoof_btn'].configure(text=current_spoof_text, fg_color=current_spoof_color)
                
                # --- LOGIC for Bind Dropdown ---
                # Compare User Selection (bind_var) vs System State (current_bind)
                user_selection = cache['bind_var'].get()
                
                if user_selection != current_bind:
                    # Divergence detected (User selected new value OR System changed)
                    # Increment persistence counter
                    cache['modified_cycles'] = cache.get('modified_cycles', 0) + 1
                    
                    if cache['modified_cycles'] >= 5:
                        # Timeout (approx 30s): Force reset to system value
                        cache['bind_var'].set(current_bind)
                        cache['modified_cycles'] = 0
                else:
                    # Synced
                    cache['modified_cycles'] = 0
                
                # Update Dropdown values (adapters) ONLY if changed
                if cache.get('last_adapters') != adapters:
                    cache['dropdown'].configure(values=adapters)
                    cache['last_adapters'] = list(adapters)

                # Rebuild buttons if shortcuts changed
                if cache['shortcuts_sig'] != shortcuts_sig:
                    for widget in cache['apps_frame'].winfo_children():
                        widget.destroy()
                    self._create_app_buttons(cache['apps_frame'], shortcuts)
                    cache['shortcuts_sig'] = shortcuts_sig

            else:
                # CREATE NEW ROW
                self._create_row(gid, box_name, shortcuts, adapters, current_bind, is_spoofed, shortcuts_sig)

    def _create_row(self, gid, box_name, shortcuts, adapters, current_bind, is_spoofed, shortcuts_sig):
        # Row Frame
        row = ctk.CTkFrame(self.scrollable)
        row.pack(fill="x", pady=2, padx=5) 
        
        # ID Label 
        lb = ctk.CTkLabel(row, text=f"#{gid} [{box_name}]", font=("Roboto", 14, "bold"), width=140, anchor="w")
        lb.pack(side="left", padx=(5, 10))
        
        # Application Buttons Container
        apps_frame = ctk.CTkFrame(row, fg_color="transparent")
        apps_frame.pack(side="left", fill="x", expand=True)
        self._create_app_buttons(apps_frame, shortcuts)

        # Right Side Container (Spoof + Bind)
        bind_frame = ctk.CTkFrame(row, fg_color="transparent")
        bind_frame.pack(side="right", padx=10)
        
        # --- Spoof Button ---
        spoof_text = "Spoofado" if is_spoofed else "Spoofar"
        spoof_color = "green" if is_spoofed else "#3B8ED0"
        
        def toggle_spoof_action(bname=box_name):
            # Check current state from button text to decide action
            # Or assume the click inverts state. 
            # Ideally we re-check real state, but UI state is faster.
            now_spoofed = (spoof_btn.cget('text') == "Spoofado")
            new_state = not now_spoofed
            
            # Update UI immediately for responsiveness
            spoof_btn.configure(text="Aplicando...", fg_color="gray")
            self.update() # Force redraw
            
            # Run Logic
            self.sandbox_manager.toggle_spoof(bname, new_state)
            
            # Validation will happen on next refresh cycle, 
            # but we can set tentative state
            txt = "Spoofado" if new_state else "Spoofar"
            clr = "green" if new_state else "#3B8ED0"
            spoof_btn.configure(text=txt, fg_color=clr)

        spoof_btn = ctk.CTkButton(bind_frame, text=spoof_text, width=80, fg_color=spoof_color, command=toggle_spoof_action)
        spoof_btn.pack(side="left", padx=(0, 10)) # Margin right to separate from dropdown

        # --- Bind Dropdown ---
        var = ctk.StringVar(value=current_bind)
        dropdown = ctk.CTkOptionMenu(bind_frame, values=adapters, variable=var, width=150)
        dropdown.pack(side="left", padx=5)
        
        def apply_bind(bname=box_name, v=var):
            sel = v.get()
            self.sandbox_manager.set_bind_adapter(bname, sel)
            print(f"Set {bname} to {sel}")
            
        ctk.CTkButton(bind_frame, text="Bind", width=60, fg_color="#3B8ED0", command=apply_bind).pack(side="left")

        # Cache references
        self.rows_cache[gid] = {
            'frame': row,
            'bind_var': var,
            'dropdown': dropdown,
            'spoof_btn': spoof_btn,
            'apps_frame': apps_frame,
            'shortcuts_sig': shortcuts_sig,
            'box_name': box_name,
            'modified_cycles': 0,
            'last_adapters': list(adapters)
        }

    def _create_app_buttons(self, parent, shortcuts):
        for s in shortcuts:
            # Shorten name mechanism
            btn_text = s.app_name.replace(".exe", "").replace(".lnk", "").capitalize()
            # If empty (from regex fallback), show filename
            if not btn_text.strip(): btn_text = s.name
            
            ctk.CTkButton(parent, text=btn_text, width=100, 
                          command=lambda p=s.path: self.sandbox_manager.launch_shortcut(p)).pack(side="left", padx=5)

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
