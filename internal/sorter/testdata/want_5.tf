resource "a" "a" {
  allowed_ports = toset([
    "http-80-tcp",
    "https-443-tcp",
    "ssh-22-tcp",
  ])
}
