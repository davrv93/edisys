#!/usr/bin/env python3
"""F4 · QLoRA del motor de EDISYS (transformers + peft, MPS/CPU de la Mac).

El dataset es pequeño (decenas de ejemplos), así que el entrenamiento en MPS
tarda minutos. Guarda el modelo fusionado en fusionado/ listo para
convert_hf_to_gguf.py de llama.cpp (lo llama entrenar-qlora.sh).

Uso:  python3 entrenar.py [dataset/edisys_chat.jsonl]
"""
import json
import sys
from pathlib import Path

import torch
from peft import LoraConfig, get_peft_model
from transformers import AutoModelForCausalLM, AutoTokenizer, Trainer, TrainingArguments

BASE = "Qwen/Qwen2.5-1.5B-Instruct"
DIR = Path(__file__).parent
DATASET = Path(sys.argv[1]) if len(sys.argv) > 1 else DIR / "dataset" / "edisys_chat.jsonl"
SALIDA = DIR / "adapters"
R, ALFA, LR, EPOCAS = 16, 32, 2e-4, 3


def main():
    from datasets import Dataset

    tok = AutoTokenizer.from_pretrained(BASE)
    if not tok.pad_token:
        tok.pad_token = tok.eos_token

    filas = [json.loads(l) for l in DATASET.read_text(encoding="utf-8").splitlines() if l.strip()]
    datos = []
    for f in filas:
        texto = tok.apply_chat_template(f["messages"], tokenize=False, add_generation_prompt=False)
        ids = tok(texto, truncation=True, max_length=1024)
        datos.append({"input_ids": ids["input_ids"],
                      "attention_mask": ids["attention_mask"],
                      "labels": list(ids["input_ids"])})
    ds = Dataset.from_list(datos)

    modelo = AutoModelForCausalLM.from_pretrained(BASE, torch_dtype=torch.float32)
    modelo = get_peft_model(modelo, LoraConfig(
        r=R, lora_alpha=ALFA, lora_dropout=0.05, bias="none",
        task_type="CAUSAL_LM", target_modules=["q_proj", "k_proj", "v_proj", "o_proj"]))
    modelo.print_trainable_parameters()

    def colador(batch):
        ancho = max(len(x) for x in batch["input_ids"])
        relleno = tok.pad_token_id
        return {
            "input_ids": [x + [relleno] * (ancho - len(x)) for x in batch["input_ids"]],
            "attention_mask": [x + [0] * (ancho - len(x)) for x in batch["attention_mask"]],
            "labels": [x + [-100] * (ancho - len(x)) for x in batch["labels"]],
        }

    Trainer(
        model=modelo,
        args=TrainingArguments(
            output_dir=str(SALIDA),
            per_device_train_batch_size=2,
            gradient_accumulation_steps=2,
            num_train_epochs=EPOCAS,
            learning_rate=LR,
            logging_steps=10,
            save_strategy="no",
            use_cpu=bool(not torch.backends.mps.is_available()),
        ),
        data_collator=colador,
        train_dataset=ds,
    ).train()

    fusion = DIR / "fusionado"
    modelo = modelo.merge_and_unload()
    modelo.save_pretrained(fusion)
    tok.save_pretrained(fusion)
    print(f"F4 · fusionado en {fusion} (listo para convert_hf_to_gguf.py)")


if __name__ == "__main__":
    main()
