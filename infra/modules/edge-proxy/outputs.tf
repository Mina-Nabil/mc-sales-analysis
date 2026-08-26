output "public_ip" { value = aws_eip.proxy.public_ip }

# CloudFront custom origin needs a domain, not a bare IP. The EIP's AWS DNS name
# resolves to it and is stable for the life of the address.
output "public_dns" { value = aws_eip.proxy.public_dns }
