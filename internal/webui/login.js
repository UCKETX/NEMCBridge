const form = document.getElementById('login-form');
const submit = document.getElementById('login-submit');
const error = document.getElementById('login-error');
form.addEventListener('submit', async event => {
  event.preventDefault();
  error.textContent = '';
  submit.disabled = true;
  submit.textContent = '正在验证…';
  try {
    const response = await fetch('/api/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        username: document.getElementById('username').value.trim(),
        password: document.getElementById('password').value
      })
    });
    if (!response.ok) throw new Error(await response.text());
    document.getElementById('password').value = '';
    window.location.replace('/');
  } catch (failure) {
    error.textContent = failure.message || '连接失败，请稍后重试。';
    submit.disabled = false;
    submit.textContent = '登录控制台 →';
  }
});
