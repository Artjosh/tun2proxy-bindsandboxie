import subprocess
import time
import re
import threading
from dataclasses import dataclass
from typing import List, Callable

@dataclass
class ProxyConfig:
    ip: str
    port: str
    username: str
    password: str
    id: int

class ProxyEngine:
    def __init__(self, tun2socks_path: str):
        self.tun2socks_path = tun2socks_path
        self._stop_flag = False

    def parse_proxies(self, text: str) -> List[ProxyConfig]:
        """Parses lines of IP:PORT:USER:PASS into ProxyConfig objects."""
        proxies = []
        lines = text.strip().split('\n')
        for i, line in enumerate(lines):
            parts = line.strip().split(':')
            if len(parts) >= 4:
                proxies.append(ProxyConfig(
                    ip=parts[0],
                    port=parts[1],
                    username=parts[2],
                    password=parts[3],
                    id=i + 1
                ))
        return proxies

    def stop_all(self):
        """Kills all tun2socks processes."""
        subprocess.run(["taskkill", "/F", "/IM", "tun2socks.exe"], 
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def start_proxies(self, proxies: List[ProxyConfig], log_callback: Callable[[str], None] = None):
        """Starts a list of proxies sequentially."""
        self.stop_all()
        
        def log(msg):
            if log_callback:
                log_callback(msg)
            else:
                print(msg)

        # Threading this to not freeze GUI
        threading.Thread(target=self._run_sequence, args=(proxies, log), daemon=True).start()

    def _run_sequence(self, proxies: List[ProxyConfig], log):
        for p in proxies:
            dev_name = f"Proxy_{p.id}"
            
            # Start tun2socks
            cmd = [
                self.tun2socks_path,
                "-device", dev_name,
                "-proxy", f"socks5://{p.username}:{p.password}@{p.ip}:{p.port}",
                "-loglevel", "error" # Reduce noise
            ]
            
            log(f"[{p.id}] Starting interface {dev_name} connected to {p.ip}...")
            # Use specific flags to hide window if possible, but subprocess.Popen is standard
            subprocess.Popen(cmd, creationflags=subprocess.CREATE_NO_WINDOW)
            
            # Wait for interface
            if not self._wait_for_interface(dev_name):
                 log(f"[{p.id}] ERROR: Interface {dev_name} failed to appear.")
                 continue

            # Configure IP with retry and verification
            local_ip = f"10.0.{p.id}.1"
            gateway = f"10.0.{p.id}.254"
            log(f"[{p.id}] Setting IP {local_ip}...")
            
            if self._set_ip(dev_name, local_ip, gateway, log=log):
                log(f"[{p.id}] Ready.")
            else:
                log(f"[{p.id}] WARNING: Interface may not work correctly!")

        log("All proxies started.")

    def _wait_for_interface(self, name: str, timeout: int = 10) -> bool:
        """Polls netsh to see if interface exists."""
        start = time.time()
        while time.time() - start < timeout:
            res = subprocess.run(
                ["netsh", "interface", "show", "interface", f"name={name}"],
                capture_output=True, text=True
            )
            if res.returncode == 0:
                return True
            time.sleep(0.5)
        return False

    def _set_ip(self, name: str, ip: str, gateway: str, log=None, max_retries: int = 3):
        """Sets the IP address with verification and retry logic."""
        for attempt in range(1, max_retries + 1):
            # set address
            subprocess.run([
                "netsh", "interface", "ip", "set", "address", 
                f"name={name}", "source=static", f"addr={ip}", 
                "mask=255.255.255.0", f"gateway={gateway}"
            ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=subprocess.CREATE_NO_WINDOW)
            
            # set metric
            subprocess.run([
                "netsh", "interface", "ip", "set", "interface", 
                name, "metric=500"
            ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=subprocess.CREATE_NO_WINDOW)
            
            # Wait a bit for IP to apply
            time.sleep(0.5)
            
            # Verify IP was applied
            if self._verify_ip(name, ip):
                return True
            
            if log:
                log(f"[{name}] IP verification failed (attempt {attempt}/{max_retries}), retrying...")
            time.sleep(1)
        
        if log:
            log(f"[{name}] ERROR: Failed to set IP {ip} after {max_retries} attempts!")
        return False
    
    def _verify_ip(self, name: str, expected_ip: str) -> bool:
        """Verifies the interface has the expected IP address using PowerShell."""
        try:
            # Use PowerShell Get-NetIPAddress which is language-independent
            ps_cmd = f'(Get-NetIPAddress -InterfaceAlias "{name}" -AddressFamily IPv4 -ErrorAction SilentlyContinue).IPAddress'
            res = subprocess.run(
                ["powershell", "-NoProfile", "-Command", ps_cmd],
                capture_output=True, text=True, creationflags=subprocess.CREATE_NO_WINDOW
            )
            actual_ip = res.stdout.strip()
            return actual_ip == expected_ip
        except Exception:
            return False

