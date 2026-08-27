import { PageBase, type Vals } from '../shared/ui/runtime';
import { readPublicSurvey, readSurveyResult, submitSurvey } from '../api/public-survey';
import type { PublicSurveyDefinition, PublicSurveyQuestion, PublicSurveyResult, PublicSurveySubmissionAnswer } from '../api/generated/health';

const validID = (value: number): boolean => Number.isSafeInteger(value) && value > 0;
const validToken = (value: string): boolean => /^[A-Za-z0-9_-]{43}$/.test(value);

export class H5Controller extends PageBase {
  private definition: PublicSurveyDefinition | null = null;
  private result: PublicSurveyResult | null = null;
  private answers = new Map<number, number[]>();
  private questionIndex = 0;
  private loading = false;
  private submitting = false;
  private error = '';
  private submissionKey = '';
  private resultToken = '';
  private submitted = false;

  constructor(readonly page: string) { super(); }

  async init(): Promise<void> {
    if (!['all', 'one', 'result'].includes(this.page)) return;
    this.loading = true;
    this.error = '';
    this.refresh();
    try {
      if (this.page === 'result') {
        const query = new URLSearchParams(location.search);
        this.resultToken ||= new URLSearchParams((location.hash || '').slice(1)).get('result_token') || query.get('result_token') || '';
        if (!validToken(this.resultToken)) throw new Error('缺少有效结果凭据，无法查询提交结果');
        const result = await readSurveyResult(this.resultToken);
        if (!validID(result.submission_id) || !validID(result.definition_version) ||
            !Number.isFinite(Date.parse(result.submitted_at)) || result.local_only !== true || result.external_executed !== false) {
          throw new Error('提交结果响应不完整，未确认结果');
        }
        this.result = result;
      } else {
        const slug = new URLSearchParams(location.search).get('slug') || '';
        if (!/^[a-z0-9][a-z0-9-]{0,119}$/.test(slug)) throw new Error('缺少有效公开问卷 slug，不能填写或提交');
        const definition = await readPublicSurvey(slug);
        this.validateDefinition(definition, slug);
        this.definition = definition;
      }
    } catch (error) {
      this.definition = null;
      this.result = null;
      this.error = error instanceof Error ? error.message : '问卷读取失败，请重试';
    } finally {
      this.loading = false;
      this.refresh();
    }
  }

  private refresh(): void { this.__render?.(); }

  private validateDefinition(definition: PublicSurveyDefinition, slug: string): void {
    if (!definition || definition.slug !== slug || !validID(definition.id) || !validID(definition.version) ||
        typeof definition.title !== 'string' || !definition.title || typeof definition.description !== 'string' ||
        !['all_in_one', 'one_by_one'].includes(definition.answer_display_mode) ||
        !Array.isArray(definition.questions) || !definition.questions.length) throw new Error('公开问卷定义不完整');
    const ids = new Set<number>();
    for (const question of definition.questions) {
      if (!validID(question.id) || ids.has(question.id) || !['single_choice', 'multi_choice'].includes(question.type) ||
          typeof question.title !== 'string' || !question.title || typeof question.required !== 'boolean' ||
          !Array.isArray(question.options) || !question.options.length ||
          !Number.isInteger(question.minimum_selections) || !Number.isInteger(question.maximum_selections) ||
          question.minimum_selections < 0 || question.maximum_selections < 1 ||
          question.minimum_selections > question.maximum_selections || question.maximum_selections > question.options.length ||
          (question.required && question.minimum_selections === 0) || (question.type === 'single_choice' && question.maximum_selections !== 1)) {
        throw new Error('问卷题目超出当前单选/多选契约');
      }
      ids.add(question.id);
      const options = new Set<number>();
      for (const option of question.options) {
        if (!validID(option.id) || options.has(option.id) || typeof option.option_text !== 'string' || !option.option_text) throw new Error('问卷选项响应不完整');
        options.add(option.id);
      }
    }
  }

  private select(question: PublicSurveyQuestion, optionID: number): void {
    if (this.submitting || this.submitted) return;
    const previous = this.answers.get(question.id) || [];
    const next = question.type === 'single_choice' ? [optionID]
      : previous.includes(optionID) ? previous.filter((id) => id !== optionID) : [...previous, optionID];
    next.sort((a, b) => a - b);
    if (next.join(',') !== previous.join(',')) this.submissionKey = '';
    this.answers.set(question.id, next);
    this.error = '';
    this.refresh();
  }

  private questionError(question: PublicSurveyQuestion): string {
    const count = (this.answers.get(question.id) || []).length;
    if (count === 0 && !question.required) return '';
    return count < question.minimum_selections || count > question.maximum_selections
      ? `「${question.title}」请选择 ${question.minimum_selections}–${question.maximum_selections} 项` : '';
  }

  private async submit(): Promise<void> {
    if (!this.definition || this.submitting || this.submitted) return;
    this.error = this.definition.questions.map((question) => this.questionError(question)).find(Boolean) || '';
    if (this.error) { this.refresh(); return; }
    const answers: PublicSurveySubmissionAnswer[] = this.definition.questions
      .filter((question) => (this.answers.get(question.id) || []).length > 0)
      .map((question) => ({ question_id: question.id, option_ids: [...this.answers.get(question.id)!] }));
    this.submitting = true;
    this.refresh();
    try {
      // One key per unchanged answer set in this filling lifecycle. Unknown
      // network outcomes retry the same request; editing answers clears the key.
      this.submissionKey ||= btoa(String.fromCharCode(...crypto.getRandomValues(new Uint8Array(32))))
        .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
      const receipt = await submitSurvey(this.definition.slug, { version: this.definition.version, submission_key: this.submissionKey, answers });
      if (!validToken(receipt.resultToken)) throw new Error('提交回执缺少有效结果凭据，未确认结果');
      this.resultToken = receipt.resultToken;
      this.submitted = true;
    } catch (error) {
      this.error = error instanceof Error ? error.message : '提交失败；未修改答案时可安全重试';
    } finally {
      this.submitting = false;
      this.refresh();
    }
  }

  private move(delta: number): void {
    if (!this.definition || this.submitting || this.submitted) return;
    if (delta > 0) {
      this.error = this.questionError(this.definition.questions[this.questionIndex]);
      if (this.error) { this.refresh(); return; }
    }
    this.questionIndex = Math.max(0, Math.min(this.definition.questions.length - 1, this.questionIndex + delta));
    this.error = '';
    this.refresh();
  }

  renderVals(): Vals {
    const definition = this.definition;
    const stepMode = definition?.answer_display_mode === 'one_by_one';
    const questions = definition?.questions || [];
    const visible = stepMode ? questions.slice(this.questionIndex, this.questionIndex + 1) : questions;
    const ready = !!definition && !this.loading && !this.submitted;
    return {
      loading: this.loading, error: this.error, ready, result: this.result,
      submitted: this.submitted, resultPath: `result.html#result_token=${encodeURIComponent(this.resultToken)}`,
      title: definition?.title || '公开问卷', description: definition?.description || '',
      progress: stepMode ? `第 ${this.questionIndex + 1} / ${questions.length} 题` : `共 ${questions.length} 题`,
      canPrevious: ready && stepMode && this.questionIndex > 0 && !this.submitting,
      canNext: ready && stepMode && this.questionIndex < questions.length - 1 && !this.submitting,
      canSubmit: ready && (!stepMode || this.questionIndex === questions.length - 1) && !this.submitting,
      submitting: this.submitting,
      canRetry: !this.loading && !!this.error && !definition,
      blockedReason: this.page === 'auth'
        ? '后端能力未就绪：H5 OAuth Provider 当前禁用，不能授权。请使用已发布的匿名问卷测试入口。'
        : '后端能力未就绪：当前页面没有可用的报名、支付、续费、二维码或完成状态契约；未执行任何外部操作。',
      questions: visible.map((question) => ({
        id: question.id, title: question.title, required: question.required ? '（必答）' : '（选答）',
        hint: `${question.type === 'single_choice' ? '单选' : '多选'} · 最少 ${question.minimum_selections} 项，最多 ${question.maximum_selections} 项`,
        options: question.options.map((option) => {
          const selected = (this.answers.get(question.id) || []).includes(option.id);
          return { id: option.id, text: option.option_text, selected, mark: selected ? '✓' : '○',
            style: { background: selected ? '#EFF4FF' : '#fff', borderColor: selected ? '#3370ff' : '#DEE0E3' },
            pick: () => this.select(question, option.id) };
        }),
      })),
      act: { submit: () => { void this.submit(); }, previous: () => this.move(-1), next: () => this.move(1), retry: () => { void this.init(); } },
    };
  }
}
