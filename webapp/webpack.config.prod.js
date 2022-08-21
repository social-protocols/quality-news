const {webProd} = require("@fun-stack/fun-pack");

// https://github.com/fun-stack/fun-pack
module.exports = webProd({
  indexHtml: "src/main/html/index.html",
  assetsDir: "assets"
});
