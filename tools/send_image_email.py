import smtplib
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
from email.mime.image import MIMEImage
import ssl, struct, zlib, sys

to_addr = sys.argv[1]
subject = sys.argv[2]
body = sys.argv[3]

msg = MIMEMultipart()
msg['From'] = 'tester@owowo.dev'
msg['To'] = to_addr
msg['Subject'] = subject
msg.attach(MIMEText(body, 'plain', 'utf-8'))

def make_png(r, g, b):
    sig = b'\x89PNG\r\n\x1a\n'
    def chunk(ctype, data):
        c = ctype + data
        return struct.pack('>I', len(data)) + c + struct.pack('>I', zlib.crc32(c) & 0xffffffff)
    ihdr = struct.pack('>IIBBBBB', 1, 1, 8, 2, 0, 0, 0)
    raw = b'\x00' + bytes([r, g, b])
    idat = zlib.compress(raw)
    return sig + chunk(b'IHDR', ihdr) + chunk(b'IDAT', idat) + chunk(b'IEND', b'')

img = MIMEImage(make_png(0, 0, 255), name='new.png')
img.add_header('Content-Disposition', 'attachment', filename='new.png')
msg.attach(img)

ctx = ssl.create_default_context()
with smtplib.SMTP_SSL('smtp.purelymail.com', 465, context=ctx) as s:
    s.login('tester@owowo.dev', 'ncjyomuulyisrnxufzqd')
    s.send_message(msg)
print('sent OK')
