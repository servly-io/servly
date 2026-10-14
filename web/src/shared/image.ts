/**
 * Shrinks a photo in the browser until it fits the server's upload limit:
 * phone cameras produce 3-8 MB, a menu photo needs far less. Returns the file
 * unchanged when it already fits, or when it cannot be made small enough (the
 * server then says so).
 */
export async function fitImage(file: File, maxBytes: number): Promise<Blob> {
  if (file.size <= maxBytes) return file;
  let bmp: ImageBitmap;
  try {
    bmp = await createImageBitmap(file);
  } catch {
    return file; // not an image the browser can read: let the server explain
  }
  for (const side of [2048, 1600, 1280, 1024, 800]) {
    const scale = Math.min(1, side / Math.max(bmp.width, bmp.height));
    const c = document.createElement('canvas');
    c.width = Math.round(bmp.width * scale);
    c.height = Math.round(bmp.height * scale);
    const g = c.getContext('2d');
    if (!g) return file;
    g.fillStyle = '#fff'; // JPEG has no transparency
    g.fillRect(0, 0, c.width, c.height);
    g.drawImage(bmp, 0, 0, c.width, c.height);
    const blob = await new Promise<Blob | null>((done) => c.toBlob(done, 'image/jpeg', 0.82));
    if (blob && blob.size <= maxBytes) return blob;
  }
  return file;
}

/** "1 MB", "512 KB". */
export function formatBytes(n: number): string {
  return n % (1 << 20) === 0 ? `${n >> 20} MB` : `${Math.round(n / 1024)} KB`;
}
