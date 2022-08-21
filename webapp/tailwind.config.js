module.exports = {
  content: ["./*.js"],
  plugins: [
    require('daisyui'),
  ],
  daisyui: {
    logs: false, // otherwise daisy logs its ui version
    themes: [
      'light',
      'dark'
    ]
  },
};
