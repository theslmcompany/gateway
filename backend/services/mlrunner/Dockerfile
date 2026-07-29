FROM python:3.12-slim

WORKDIR /app

# Install deps first (cached layer)
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy config and registry — needed for model pre-download
COPY analyzers.yml .
COPY registry.py .

# Pre-download all models declared in default config (baked into image).
# Override at runtime with ANALYZERS_CONFIG to load a different set.
RUN python -c "from registry import load_registry; load_registry()"

# Copy remaining source
COPY . .

EXPOSE 8082

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8082"]
