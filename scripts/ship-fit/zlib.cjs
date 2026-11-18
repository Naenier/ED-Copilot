const { Buffer } = require('buffer');
const { gzip, Inflate } = require('pako');

exports.gzipSync = bytes => Buffer.from(gzip(bytes));

exports.gunzipSync = bytes => {
  const chunks = [];
  const inflator = new Inflate({ chunkSize: 8192 });
  let size = 0;
  inflator.onData = chunk => {
    size += chunk.length;
    if (size > 32768) throw new Error('Invalid ship build');
    chunks.push(Buffer.from(chunk));
  };
  inflator.push(bytes, true);
  if (inflator.err) throw new Error(inflator.msg);
  return Buffer.concat(chunks);
};
