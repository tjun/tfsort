resource "b" "b" {
  security_group_rules = [
    { type = "ingress", from_port = 22, protocol = "tcp" },
    "http-80-tcp", # HTTP traffic
    1024,          # Custom port
    "ssh-22-tcp",  # SSH access
  ]
}

resource "a" "a" {
  example_list = [
    # Group A
    "charlie",
    "alpha",

    # Group B
    "bravo",
  ]
}
